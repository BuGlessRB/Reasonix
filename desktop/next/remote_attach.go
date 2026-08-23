package main

import (
	"context"
	"os"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/releaseasset"
	"reasonix/internal/remote"
	"reasonix/internal/remote/attach"
	"reasonix/internal/serve"
)

// remoteLink gives the hub what it needs for panes on other machines. Prompts
// are left unset for now: a first-seen host key and a typed passphrase both
// need a dialog this window does not have yet, and refusing is the safe
// direction — a host already in known_hosts, reached by key or agent, connects.
type remoteLink struct{ pool *attach.Pool }

func newRemoteLink(ctx context.Context) *remoteLink {
	return &remoteLink{pool: attach.NewPool(ctx, attach.Options{
		LocalBinary: currentExecutable(),
		Version:     version,
		FetchBinary: fetchRemoteBinary,
	})}
}

func (r *remoteLink) Attach(ctx context.Context, host, workspace string) (serve.RemoteEndpoint, func(), error) {
	ep, err := r.pool.Attach(ctx, host, workspace, attach.Call{})
	if err != nil {
		return serve.RemoteEndpoint{}, nil, err
	}
	return serve.RemoteEndpoint{
		Host:      ep.Host,
		Workspace: ep.Workspace,
		Addr:      ep.Addr,
		Token:     ep.Token,
	}, ep.Release, nil
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

func currentExecutable() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return ""
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
