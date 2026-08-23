package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
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
	if h.opts.AttachRemote == nil {
		refuse(w, http.StatusNotImplemented, "remote.not_available",
			"this kernel does not open panes on other machines", nil)
		return
	}
	ep, release, err := h.opts.AttachRemote(r.Context(), req.Host, req.Workspace)
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
