package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"reasonix/internal/agent"
	"reasonix/internal/billing"
	"reasonix/internal/boot"
	"strconv"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

// decorateSink applies the host's sink wrapper, if it asked for one.
func (h *Hub) decorateSink(sink event.Sink) event.Sink {
	if h.opts.DecorateSink == nil {
		return sink
	}
	return h.opts.DecorateSink(sink)
}

// runtimePrefix is where a hub publishes its runtimes. The frontend builds
// every request under it, so it is part of the wire contract.
const runtimePrefix = "/rt/"

// maxRuntimesDefault caps concurrently driven sessions: each runtime is a full
// assembly — tools, extensions, MCP sidecars — so an unbounded pane count is an
// unbounded process. Where the ceiling belongs depends on the machine, so
// [desktop] max_panes moves it, up to 32.
const maxRuntimesDefault = 8

// maxRuntimes reads the ceiling for this machine. Opening a pane is not a hot
// path, so it is read per call rather than cached into a stale number.
func maxRuntimes() int {
	cfg, err := config.Load()
	if err != nil {
		return maxRuntimesDefault
	}
	return cfg.DesktopMaxPanes(maxRuntimesDefault)
}

// Hub serves several sessions at the same time. Each Runtime is a complete
// Server with its own controller, event stream, title cache and session lease,
// published under /rt/{id}/; the hub owns only what they share — the auth gate,
// the workspace tree, and the rule that one session file gets one runtime.
type Hub struct {
	mu       sync.RWMutex
	runtimes map[string]*Runtime
	order    []string
	seq      int

	opts HubOptions
	auth *authGate
	// What the host decided once and every later pane must inherit: where the
	// setup surface is allowed, and the context its recovery sweep rides.
	setupAddr string
	gcCtx     context.Context
	// titles reads each project's cached session titles for the sidebar tree.
	titles map[string]*titleCache
	// wallets is shared by every pane: opening a conversation builds a runtime,
	// and a per-runtime cache would be cold on exactly the read a switch waits on.
	wallets *billing.Store
}

// Runtime is one open session: the server driving it, the stream it emits, and
// the workspace it was opened against.
type Runtime struct {
	ID     string
	Root   string
	Server *Server
	Events *Broadcaster

	// Set when a kernel on another machine drives this pane: Server and Events
	// are nil then, and the handler proxies rather than routes.
	remote *remoteBinding

	handler http.Handler
	leases  *control.SessionLeaseKeeper
	stop    context.CancelFunc
}

// HubOptions carries what the embedding host decides for every runtime.
type HubOptions struct {
	Serve config.ServeConfig
	// Grant applies the host's capabilities (folder picking, provider edits) to
	// each runtime, so a pane opened later can do what the first one could.
	Grant func(*Server)
	// OnOpen and OnClose let a host attach its own transport to a runtime — the
	// Wails shell pumps each one's frames onto its bus, keyed by ID.
	OnOpen  func(*Runtime)
	OnClose func(*Runtime)
	// DecorateSink wraps each runtime's event sink, the way Grant applies its
	// capabilities. A window adds system notifications here; a networked server
	// leaves it nil, or they fire on the kernel's machine, not the watcher's.
	DecorateSink func(event.Sink) event.Sink
	// Remote reaches workspaces on other machines. Nil refuses them: a server
	// that dials onward on a request's say-so is someone else's way in.
	Remote RemoteAttacher
}

// OpenRequest asks for a runtime. An empty SessionPath opens a fresh session in
// Root; a path that is already open focuses that runtime rather than binding a
// second writer to one transcript.
type OpenRequest struct {
	Root        string `json:"root"`
	SessionPath string `json:"sessionPath"`
	Model       string `json:"model"`
}

// RuntimeView is what the frontend needs to address and label a pane.
type RuntimeView struct {
	ID          string `json:"id"`
	Base        string `json:"base"`
	Root        string `json:"root"`
	Name        string `json:"name"`
	SessionPath string `json:"sessionPath,omitempty"`
	// Set only on a pane driven over SSH. Its absence is what tells the
	// frontend the pane is this machine's own — the common case stays unmarked.
	Host string `json:"host,omitempty"`
}

// NewHub returns an empty hub. Adopt or Open publishes the first runtime.
func NewHub(opts HubOptions) *Hub {
	return &Hub{
		runtimes: map[string]*Runtime{},
		opts:     opts,
		auth:     newAuthGate(opts.Serve),
		wallets:  &billing.Store{},
	}
}

// AuthToken and AuthMode report the gate every runtime shares, so a host still
// prints one token rather than one per pane.
func (h *Hub) AuthToken() string { return h.auth.Token() }

// AuthMode reports the shared gate's mode.
func (h *Hub) AuthMode() string { return h.auth.Mode() }

// Adopt takes over a runtime the host assembled before the hub existed — the
// window's first session — and publishes it under an ID.
func (h *Hub) Adopt(srv *Server, bc *Broadcaster) (*Runtime, error) {
	if srv == nil {
		return nil, nil
	}
	srv.auth = h.auth
	if h.opts.Grant != nil {
		h.opts.Grant(srv)
	}
	leases, err := h.ownSession(srv)
	if err != nil {
		return nil, err
	}
	rt := &Runtime{ID: h.nextID(), Root: srv.Controller().WorkspaceRoot(), Server: srv, Events: bc, leases: leases}
	h.publish(rt)
	return rt, nil
}

// ownSession makes "a runtime owns the session it writes" true however the
// runtime was born. A host that arranged ownership itself keeps it; anything
// else is given a keeper here — before it holds anything, since a window opens
// with no session and the first turn mints one. SetOnSessionPathChanged carries
// the lease onto that path when it appears.
func (h *Hub) ownSession(srv *Server) (keeper *control.SessionLeaseKeeper, err error) {
	if srv.leases != nil {
		return nil, nil // the host's, and the host's to release
	}
	leases := control.NewSessionLeaseKeeper()
	// A refusal must leave the server as it was found, so the caller can decide
	// on another session and adopt again.
	defer func() {
		if err != nil {
			_ = srv.SetSessionLeases(nil)
			leases.Release()
		}
	}()
	if err = srv.SetSessionLeases(leases); err != nil {
		return nil, err
	}
	if path := strings.TrimSpace(srv.Controller().SessionPath()); path != "" {
		if err = srv.rebindSessionLease(path); err != nil {
			return nil, err
		}
	}
	return leases, nil
}

// Open builds a runtime for req, or returns the one already driving that
// session. The caller gets a runtime it can address immediately.
func (h *Hub) Open(ctx context.Context, req OpenRequest) (*Runtime, error) {
	if rt := h.findSession(req.SessionPath); rt != nil {
		return rt, nil
	}
	limit := maxRuntimes()
	h.mu.RLock()
	full := len(h.order) >= limit
	h.mu.RUnlock()
	if full {
		return nil, refusal(http.StatusConflict, "hub.too_many_panes",
			fmt.Errorf("already driving %d sessions — close one first", limit), map[string]any{"max": limit})
	}
	root, err := h.resolveRoot(req)
	if err != nil {
		return nil, err
	}
	bc := NewBroadcaster()
	built, err := boot.BuildRuntime(ctx, boot.Options{
		Model:         strings.TrimSpace(req.Model),
		WorkspaceRoot: root,
		SessionDir:    SessionDirFor(root),
		Sink:          h.decorateSink(bc),
		Stderr:        os.Stderr,
		StatsSource:   "serve",
		BalanceStore:  h.wallets,
	})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", root, err)
	}
	srv := New(built.Controller, bc, h.opts.Serve)
	srv.AdoptRuntime(built)
	srv.auth = h.auth
	if h.opts.Grant != nil {
		h.opts.Grant(srv)
	}
	built.Controller.EnableInteractiveApproval()
	// Ask/Auto/YOLO is a posture the user set on the window, not a per-session
	// default, so a pane opened later starts where the others already are.
	if mode := h.approvalPosture(); mode != "" {
		built.Controller.SetToolApprovalMode(mode)
	}
	// Own the session file for as long as this pane lives, so a second pane —
	// or another process — is refused instead of silently double-writing.
	leases := control.NewSessionLeaseKeeper()
	if err := srv.SetSessionLeases(leases); err != nil {
		built.Controller.Close()
		leases.Release()
		return nil, err
	}
	if path := strings.TrimSpace(req.SessionPath); path != "" {
		if _, err := srv.resumeInto(path); err != nil {
			built.Controller.Close()
			leases.Release()
			return nil, err
		}
	}
	rt := &Runtime{ID: h.nextID(), Root: root, Server: srv, Events: bc, leases: leases}
	// Before publishing: Close reads rt.stop, and a pane must not be reachable
	// before the fields its teardown depends on are set.
	h.adoptHostDecisions(rt)
	h.publish(rt)
	rememberWorkspace(root)
	return rt, nil
}

// Close retires a runtime: the conversation is persisted, the assembly torn
// down, and the session file released for another window to open.
func (h *Hub) Close(id string) error {
	h.mu.Lock()
	rt := h.runtimes[id]
	if rt == nil {
		h.mu.Unlock()
		return fmt.Errorf("no runtime %s", id)
	}
	delete(h.runtimes, id)
	h.order = removeString(h.order, id)
	h.mu.Unlock()

	if h.opts.OnClose != nil {
		h.opts.OnClose(rt)
	}
	if rt.stop != nil {
		rt.stop()
	}
	if rt.remote != nil {
		// Retired before the forward that reaches it goes away: releasing first
		// would strand a session lease over there, with nothing left that could
		// reach the kernel holding it.
		rt.closeFarRuntime()
		if rt.remote.release != nil {
			rt.remote.release()
		}
		return nil
	}
	if err := rt.Server.Controller().Snapshot(); err != nil {
		slog.Warn("serve: snapshot before closing runtime", "id", id, "err", err)
	}
	rt.Server.Controller().Close()
	if rt.leases != nil {
		rt.leases.Release()
	}
	return nil
}

// Shutdown closes every runtime, newest first.
func (h *Hub) Shutdown() {
	for _, view := range h.List() {
		if err := h.Close(view.ID); err != nil {
			slog.Warn("serve: close runtime", "id", view.ID, "err", err)
		}
	}
}

// Get returns a runtime by ID, or nil.
func (h *Hub) Get(id string) *Runtime {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.runtimes[id]
}

// List returns the open runtimes in the order they were opened.
func (h *Hub) List() []RuntimeView {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]RuntimeView, 0, len(h.order))
	for _, id := range h.order {
		if rt := h.runtimes[id]; rt != nil {
			out = append(out, rt.view())
		}
	}
	return out
}

// Local reports whether this pane's kernel runs in this process. A remote one
// has no Server, Events or lease here — they belong to the machine it proxies
// to, which also already made every decision a host would apply to a pane.
func (rt *Runtime) Local() bool { return rt.remote == nil }

// localRuntimes returns the panes this process drives itself. Host decisions —
// the approval posture, the setup surface, the recovery sweep — reach those
// only: the rest are another kernel's, and reaching into one would nil-panic.
func (h *Hub) localRuntimes() []*Runtime {
	out := make([]*Runtime, 0, len(h.order))
	for _, rt := range h.Runtimes() {
		if rt.Local() {
			out = append(out, rt)
		}
	}
	return out
}

// Runtimes returns the live runtimes, for a host that needs more than the view.
func (h *Hub) Runtimes() []*Runtime {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Runtime, 0, len(h.order))
	for _, id := range h.order {
		if rt := h.runtimes[id]; rt != nil {
			out = append(out, rt)
		}
	}
	return out
}

func (rt *Runtime) view() RuntimeView {
	if rt.remote != nil {
		return rt.remoteView()
	}
	ctrl := rt.Server.Controller()
	return RuntimeView{
		ID:          rt.ID,
		Base:        runtimePrefix + rt.ID,
		Root:        ctrl.WorkspaceRoot(),
		Name:        filepath.Base(ctrl.WorkspaceRoot()),
		SessionPath: ctrl.SessionPath(),
	}
}

// Handler routes hub endpoints and mounts every runtime under /rt/{id}/. Auth,
// CSRF and logging wrap the lot once, which is why runtimes are mounted as
// bare muxes.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /runtimes", h.listRuntimes)
	mux.HandleFunc("POST /runtimes", h.openRuntime)
	mux.HandleFunc("POST /runtimes/{id}/close", h.closeRuntime)
	mux.HandleFunc("GET /remotes", h.listRemoteHosts)
	mux.HandleFunc("POST /remotes", h.saveRemoteHost)
	mux.HandleFunc("GET /remotes/candidates", h.remoteCandidates)
	mux.HandleFunc("POST /remotes/remove", h.removeRemoteHost)
	mux.HandleFunc("POST /remotes/open", h.openRemoteRuntime)
	h.registerTreeRoutes(mux)
	mux.HandleFunc(runtimePrefix+"{id}/", h.routeRuntime)
	mux.HandleFunc("/", h.routeDefault)
	return logMiddleware(h.auth.middleware(csrfGuard(mux)))
}

// The ceiling rides the list rather than a second endpoint: a client that
// hardcoded it would grey out its control at the wrong count.
func (h *Hub) listRuntimes(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Panes-Max", strconv.Itoa(maxRuntimes()))
	writeJSON(w, h.List())
}

func (h *Hub) openRuntime(w http.ResponseWriter, r *http.Request) {
	var req OpenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badBody(w)
		return
	}
	rt, err := h.Open(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, rt.view())
}

func (h *Hub) closeRuntime(w http.ResponseWriter, r *http.Request) {
	if err := h.Close(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Hub) routeRuntime(w http.ResponseWriter, r *http.Request) {
	rt := h.Get(r.PathValue("id"))
	if rt == nil {
		notFound(w, "runtime", r.PathValue("rt"))
		return
	}
	rt.handler.ServeHTTP(w, r)
}

// routeDefault keeps an unprefixed client working — a browser opened straight
// at the port, or an older frontend — by serving the first runtime.
func (h *Hub) routeDefault(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	var rt *Runtime
	if len(h.order) > 0 {
		rt = h.runtimes[h.order[0]]
	}
	h.mu.RUnlock()
	if rt == nil {
		refuse(w, http.StatusServiceUnavailable, "hub.no_runtime_open", "no runtime is open", nil)
		return
	}
	rt.Server.routes().ServeHTTP(w, r)
}

// publish registers a runtime and freezes the handler that serves it. The
// prefix is stripped here so the runtime's own routes stay unprefixed.
func (h *Hub) publish(rt *Runtime) {
	if rt.remote != nil {
		rt.handler = http.StripPrefix(runtimePrefix+rt.ID, remoteProxy(rt.remote.ep))
	} else {
		rt.handler = http.StripPrefix(runtimePrefix+rt.ID, rt.Server.routes())
	}
	h.mu.Lock()
	h.runtimes[rt.ID] = rt
	h.order = append(h.order, rt.ID)
	h.mu.Unlock()
	if h.opts.OnOpen != nil {
		h.opts.OnOpen(rt)
	}
}

// findSession returns the runtime already driving path. Session paths reach us
// spelled more than one way, so compare them canonically — binding a second
// writer to one transcript is what forks a recovery branch on every save.
func (h *Hub) findSession(path string) *Runtime {
	path = agent.CanonicalSessionPath(strings.TrimSpace(path))
	if path == "" {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, id := range h.order {
		rt := h.runtimes[id]
		// A remote transcript names a file on another machine: it cannot
		// collide with a local one, and deduplicating it is that kernel's job.
		if rt != nil && rt.remote == nil && agent.CanonicalSessionPath(rt.Server.Controller().SessionPath()) == path {
			return rt
		}
	}
	return nil
}

// resolveRoot decides which folder a new runtime opens against: the one asked
// for, the one that owns the session being opened, or the first runtime's.
func (h *Hub) resolveRoot(req OpenRequest) (string, error) {
	if root := strings.TrimSpace(req.Root); root != "" {
		return resolveWorkspaceDir(root)
	}
	if path := strings.TrimSpace(req.SessionPath); path != "" {
		if root := workspaceRootForSession(path); root != "" {
			return root, nil
		}
	}
	h.mu.RLock()
	first := ""
	if len(h.order) > 0 {
		if rt := h.runtimes[h.order[0]]; rt != nil {
			first = rt.Server.Controller().WorkspaceRoot()
		}
	}
	h.mu.RUnlock()
	if first != "" {
		return first, nil
	}
	// Closing the last pane leaves nothing to infer from, and "open a session"
	// is exactly what someone does next. Fall back to the remembered list, the
	// same answer the window uses when it launches with no pane at all.
	for _, dir := range Workspaces() {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir, nil
		}
	}
	return "", errors.New("no workspace to open in — add a folder first")
}

// approvalPosture reports the stance the window's first pane is running under.
func (h *Hub) approvalPosture() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.order) == 0 {
		return ""
	}
	rt := h.runtimes[h.order[0]]
	if rt == nil {
		return ""
	}
	return rt.Server.Controller().ToolApprovalMode()
}

func (h *Hub) nextID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	return fmt.Sprintf("r%d", h.seq)
}

func removeString(values []string, drop string) []string {
	out := values[:0]
	for _, v := range values {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}
