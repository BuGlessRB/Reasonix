package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"

	"reasonix/internal/config"
)

// RemoteEndpoint is a `reasonix serve` on another machine, already reachable at
// a local address — the near end of an SSH forward. The hub takes the endpoint
// and not the connection: what makes a pane remote is where its HTTP goes, so
// SSH stays above this file and these panes stay testable over httptest.
type RemoteEndpoint struct {
	Host      string // configured host name, shown on the pane
	Workspace string // remote workspace path, as the remote kernel spells it
	Addr      string // local end of the forward, host:port
	Token     string // what the remote serve's token gate expects
}

func (ep RemoteEndpoint) validate() error {
	switch {
	case strings.TrimSpace(ep.Host) == "":
		return errors.New("remote endpoint needs a host name")
	case strings.TrimSpace(ep.Addr) == "":
		return errors.New("remote endpoint needs a local address")
	case strings.TrimSpace(ep.Workspace) == "":
		return errors.New("remote endpoint needs a workspace path")
	}
	return nil
}

// remoteBinding is what a proxied pane holds where a local one holds an
// assembly. Grouped rather than spread across Runtime so that "is this pane
// remote" is one question with one answer.
type remoteBinding struct {
	ep RemoteEndpoint
	// release drops this pane's hold on the connection its endpoint rides.
	// Several panes share one, so the last one out closes it, not the first.
	release func()
}

// RemoteAttacher is what a host offers the hub for panes on other machines: a
// way to make one reachable, and what state each machine's link is in. The
// book of hosts stays here — configuration is this layer's, and keeping the
// attacher to the link is what keeps SSH out of the hub.
type RemoteAttacher interface {
	Attach(ctx context.Context, host, workspace string) (RemoteEndpoint, func(), error)
	// States is the live state per host name. A host with no link is absent
	// rather than reported idle: only the caller knows which hosts exist.
	States() map[string]RemoteLinkState
}

// RemoteLinkState is one machine's link as the attacher sees it.
type RemoteLinkState struct {
	Status  string // idle|connecting|connected|reconnecting|degraded|stopped
	Attempt int    // reconnect counter, while reconnecting
	Step    string // bootstrap step, while a first connect runs
	Detail  string
	Err     string
	Panes   int
}

// RemoteHostView is one row of the host book, with whatever its link is doing.
type RemoteHostView struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	Workspace string `json:"workspace,omitempty"`
	Status    string `json:"status"`
	Attempt   int    `json:"attempt,omitempty"`
	Step      string `json:"step,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	Panes     int    `json:"panes,omitempty"`
}

func (h *Hub) listRemoteHosts(w http.ResponseWriter, _ *http.Request) {
	if h.opts.Remote == nil {
		refuse(w, http.StatusNotImplemented, "remote.not_available",
			"this kernel does not open panes on other machines", nil)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	live := h.opts.Remote.States()
	out := make([]RemoteHostView, 0, len(cfg.Remote.Hosts))
	for _, entry := range cfg.Remote.Hosts {
		view := RemoteHostView{
			Name:      entry.Name,
			Target:    remoteTarget(entry),
			Workspace: entry.Workspace,
			Status:    "idle",
		}
		if state, ok := live[entry.Name]; ok {
			view.Status, view.Attempt = state.Status, state.Attempt
			view.Step, view.Detail, view.Error, view.Panes = state.Step, state.Detail, state.Err, state.Panes
		}
		out = append(out, view)
	}
	writeJSON(w, out)
}

// remoteTarget spells a host the way the user would type it to ssh. The port
// is shown only when it is not the one ssh would have assumed.
func remoteTarget(entry config.RemoteHostEntry) string {
	target := entry.Host
	if entry.User != "" {
		target = entry.User + "@" + target
	}
	if entry.Port > 0 && entry.Port != 22 {
		target += ":" + strconv.Itoa(entry.Port)
	}
	return target
}

// OpenRemoteRequest asks for a pane on another machine. An empty Workspace
// takes the host entry's own default, which the attach layer resolves.
type OpenRemoteRequest struct {
	Host      string `json:"host"`
	Workspace string `json:"workspace"`
}

func (h *Hub) openRemoteRuntime(w http.ResponseWriter, r *http.Request) {
	var req OpenRemoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badBody(w)
		return
	}
	if h.opts.Remote == nil {
		refuse(w, http.StatusNotImplemented, "remote.not_available",
			"this kernel does not open panes on other machines", nil)
		return
	}
	ep, release, err := h.opts.Remote.Attach(r.Context(), req.Host, req.Workspace)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	rt, err := h.OpenRemote(ep, release)
	if err != nil {
		release()
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, rt.view())
}

// OpenRemote publishes a pane driven by a kernel on another machine. release is
// called once when the pane closes.
func (h *Hub) OpenRemote(ep RemoteEndpoint, release func()) (*Runtime, error) {
	if err := ep.validate(); err != nil {
		return nil, err
	}
	limit := maxRuntimes()
	h.mu.RLock()
	full := len(h.order) >= limit
	h.mu.RUnlock()
	if full {
		return nil, refusal(http.StatusConflict, "hub.too_many_panes",
			fmt.Errorf("already driving %d sessions — close one first", limit), map[string]any{"max": limit})
	}
	rt := &Runtime{
		ID:     h.nextID(),
		Root:   ep.Workspace,
		remote: &remoteBinding{ep: ep, release: release},
	}
	h.publish(rt)
	return rt, nil
}

// Remote reports the endpoint a proxied pane is bound to, so a host that has
// to reach the far kernel itself — a shell pumping its stream onto a bus the
// page can hear — can do so without a second way to address it.
func (rt *Runtime) Remote() (RemoteEndpoint, bool) {
	if rt.remote == nil {
		return RemoteEndpoint{}, false
	}
	return rt.remote.ep, true
}

// remoteView labels a proxied pane. The workspace belongs to the remote
// filesystem, so its name is cut with path and not filepath: which separator
// applies is the remote kernel's to say, and V1 bootstraps POSIX hosts.
func (rt *Runtime) remoteView() RuntimeView {
	ep := rt.remote.ep
	return RuntimeView{
		ID:   rt.ID,
		Base: runtimePrefix + rt.ID,
		Root: ep.Workspace,
		Name: path.Base(strings.TrimRight(ep.Workspace, "/")),
		Host: ep.Host,
		// The transcript this pane drives is the remote controller's to name,
		// and asking costs a round trip the list cannot wait on. The sidebar
		// learns it from the pane's own status instead.
	}
}

// remoteProxy forwards a pane's requests to the remote kernel.
func remoteProxy(ep RemoteEndpoint) http.Handler {
	target := &url.URL{Scheme: "http", Host: ep.Addr}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			// The window's own cookies mean nothing to the remote gate, and the
			// token belongs in a header rather than the URL for the reason the
			// gate itself prefers one: request lines end up in logs.
			pr.Out.Header.Set("Cookie", cookieToken+"="+ep.Token)
		},
		// Go already flushes text/event-stream, but a stream that carries a
		// length would sit in the buffer — and a turn's output is worth more
		// than the syscalls saved on the occasional download.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			// A dotted code, not a proxy's default 502 page: the frontend has to
			// tell "the link is down" from "the kernel refused" to say anything
			// useful, and a bare status cannot carry which host went quiet.
			refuse(w, http.StatusBadGateway, "remote.unreachable",
				"the remote kernel is not answering", map[string]any{"host": ep.Host})
		},
	}
}
