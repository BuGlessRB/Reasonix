package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/releaseasset"
	"reasonix/internal/remote"
	"reasonix/internal/remote/attach"
	"reasonix/internal/remote/bootstrap"
	"reasonix/internal/serve"
)

// remoteLink gives the hub what it needs for panes on other machines: a pool
// that dials, and the prompts a first connect may have to stop for.
type remoteLink struct{ pool *attach.Pool }

// attachPrompts is what answers a credential or host-key question. Aliased so
// the bridge that implements it does not import the link layer to name it.
type attachPrompts = attach.Prompts

func newRemoteLink(ctx context.Context, prompts attachPrompts) *remoteLink {
	return &remoteLink{pool: attach.NewPool(ctx, attach.Options{
		Prompts: prompts,
		Version: version,
		// No LocalBinary: this executable is the window, not the CLI, and
		// uploading it would spend the transfer to have the far side reject a
		// binary with no serve command. npm, then a verified release download.
		FetchBinary: fetchRemoteBinary,
	})}
}

func (r *remoteLink) Attach(ctx context.Context, host, workspace string) (serve.RemoteEndpoint, func(), error) {
	ep, err := r.pool.Attach(ctx, host, workspace, attach.Call{})
	if err != nil {
		return serve.RemoteEndpoint{}, nil, identify(host, err)
	}
	return serve.RemoteEndpoint{
		Host:      ep.Host,
		Workspace: ep.Workspace,
		Addr:      ep.Addr,
		Token:     ep.Token,
	}, ep.Release, nil
}

// Browse lists folders over the connection alone. The pool holds the link for
// a moment afterwards, so walking down a tree is one login rather than one per
// step — and nothing is installed on the far side to answer it, which is what
// lets a folder be chosen before that machine has ever run a kernel.
func (r *remoteLink) Browse(ctx context.Context, host, dir string) (serve.RemoteListing, error) {
	listing, err := r.pool.Browse(ctx, host, dir)
	if err != nil {
		return serve.RemoteListing{}, identifyBrowse(host, dir, err)
	}
	out := serve.RemoteListing{Path: listing.Path, Parent: listing.Parent, Truncated: listing.Truncated}
	out.Folders = make([]serve.RemoteFolder, 0, len(listing.Folders))
	for _, f := range listing.Folders {
		out.Folders = append(out.Folders, serve.RemoteFolder{Name: f.Name, Path: f.Path})
	}
	return out, nil
}

// identifyBrowse separates what the reader fixes by typing a different path
// from what they fix by looking at the connection. Only this side sees the file
// protocol's answer, and a mistyped folder arriving as a bad gateway sends
// someone to check a link that is up.
func identifyBrowse(host, dir string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return serve.Refusal(http.StatusNotFound, "remote.no_such_folder", err,
			map[string]any{"host": host, "path": dir})
	case errors.Is(err, fs.ErrPermission):
		return serve.Refusal(http.StatusForbidden, "remote.folder_unreadable", err,
			map[string]any{"host": host, "path": dir})
	}
	return identify(host, err)
}

// Candidates reads the machine's own ssh_config. Filling the book by hand when
// the addresses are already written down next door is the step people skip.
func (r *remoteLink) Candidates() []string {
	src, err := remote.LoadUserSSHConfig()
	if err != nil || src == nil {
		return nil
	}
	out := make([]string, 0)
	for _, cand := range src.Aliases() {
		out = append(out, cand.Alias)
	}
	return out
}

func (r *remoteLink) States() map[string]serve.RemoteLinkState {
	live := r.pool.States()
	out := make(map[string]serve.RemoteLinkState, len(live))
	for host, st := range live {
		out[host] = serve.RemoteLinkState{
			Status:  st.Status.String(),
			Attempt: st.Attempt,
			Step:    st.Step,
			Detail:  st.Detail,
			Err:     st.Err,
			Panes:   st.Panes,
		}
	}
	return out
}

// fetchRemoteBinary downloads the release for a remote's platform when the host
// has no reasonix and cannot install one itself.
func fetchRemoteBinary(ctx context.Context, version, goos, goarch string) ([]byte, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	client, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{
		ResponseHeaderTimeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	client.Timeout = 2 * time.Minute
	return releaseasset.DownloadCLI(ctx, client, version, goos, goarch)
}

// identify gives the failures a person can act on an identity of their own. A
// changed host key is the one that must never arrive as a network error: the
// record it contradicts is what makes it checkable, and there is deliberately
// no path from here to connecting anyway.
func identify(host string, err error) error {
	var mismatch *remote.HostKeyMismatchError
	if errors.As(err, &mismatch) {
		params := map[string]any{"host": mismatch.Host, "fingerprint": mismatch.PresentedFingerprint}
		if len(mismatch.Locations) > 0 {
			params["file"] = mismatch.Locations[0].Filename
			params["line"] = mismatch.Locations[0].Line
		}
		return serve.Refusal(http.StatusConflict, "remote.host_key_changed", err, params)
	}
	var tooOld *bootstrap.KernelTooOldError
	switch {
	case errors.As(err, &tooOld):
		// The machine has a reasonix; it is from a line with no pane hub, so it
		// would answer everything a pane asked with 405. Caught here rather
		// than there: this is the side that read its version.
		return serve.Refusal(http.StatusBadGateway, "remote.kernel_too_old", err, map[string]any{"host": host})
	case errors.Is(err, remote.ErrHostKeyRejected):
		return serve.Refusal(http.StatusForbidden, "remote.host_key_rejected", err, nil)
	case errors.Is(err, remote.ErrAuthFailed):
		return serve.Refusal(http.StatusUnauthorized, "remote.auth_failed", err, nil)
	case errors.Is(err, bootstrap.ErrUnsupportedRemote):
		// The machine answered; it just is not one a kernel can be installed
		// onto. No detail: what it said is its own shell's complaint, in its
		// own code page, and pasting that on screen explains nothing.
		return serve.Refusal(http.StatusNotImplemented, "remote.unsupported_os", err, map[string]any{"host": host})
	}
	// Everything else keeps its text, which without a code reaches the window
	// as a bare status — and a bare 502 reads as "the request never arrived"
	// when what actually happened is on the other end of the link.
	return serve.Refusal(http.StatusBadGateway, "remote.attach_failed", err,
		map[string]any{"host": host, "detail": err.Error()})
}
