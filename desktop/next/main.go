// Command reasonix-studio is the Wails shell for frontend-next. It differs from
// the existing desktop binary in one way that matters: the UI talks to the
// kernel over internal/serve's HTTP surface instead of Wails bindings, so the
// same build runs in a browser against `reasonix serve`.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/desktop/internal/update"
	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/serve"

	// Kinds register from init, so a binary builds only what it links. Without
	// these the shell answers every Anthropic model with "unknown kind" at
	// switch time; openai alone arrived, pulled in transitively by config.
	_ "reasonix/internal/provider/anthropic"
	_ "reasonix/internal/provider/openai"
	_ "reasonix/internal/provider/responses"
)

// Everything the SPA is allowed to route to the kernel. Anything else falls
// through to the static assets, so an unknown path renders index.html rather
// than reaching the controller.
// Must match the name and path SsePort uses when it detects the Wails runtime.
const (
	wailsEventName = "rx:event"
	replayPath     = "/rx-replay"
	// Must match runtimePrefix in internal/serve and the frontend's pane base.
	runtimePrefix = "/rt/"
)

var apiPaths = map[string]bool{
	"/events": true, "/history": true, "/context": true, "/status": true,
	"/sessions": true, "/checkpoints": true, "/branches": true, "/models": true,
	"/submit": true, "/cancel": true, "/approve": true, "/answer": true,
	"/plan": true, "/preset": true, "/model": true, "/effort": true,
	"/goal": true, "/resume": true, "/compact": true, "/new": true,
	"/rewind": true, "/fork": true, "/summarize": true, "/forget": true,
	"/tool-approval-mode": true, "/auto-approve-tools": true, "/bypass": true,
	"/provider-setup": true, "/delete-session": true, "/inbox": true, "/inbox/items": true,
	"/trajectory": true, "/slash": true, "/complete": true,
	"/workspace": true, "/workspaces": true,
	"/mcp": true, "/skills": true, "/account": true, "/hooks": true,
	"/memory": true, "/network": true, "/shell": true, "/todos": true, "/providers": true,
	"/changes": true, "/attachments": true, "/roles": true,
	"/themes": true, "/extensions": true, "/plugins": true, "/surfaces": true,
	"/welcome": true, "/appearance": true,
	"/permissions": true, "/sandbox": true,
}

// A sub-path belongs to the resource it hangs off: /mcp/reconnect is the same
// surface as /mcp. Listing families rather than every leaf is what keeps a new
// endpoint from silently answering with index.html instead of JSON — and
// TestEveryPathTheFrontendCallsIsRouted is what keeps this list honest, because
// the comment alone did not.
var apiPrefixes = []string{"/mcp/", "/skills/", "/inbox/", "/account/", "/hooks/", "/memory/", "/network/", "/providers/", "/rewind/", "/extensions/", "/themes/", "/plugins/", "/appearance/"}

// splitRuntimePath separates a pane's address from the route it is asking for:
// /rt/r2/status is runtime r2 asking for /status. An unprefixed path belongs to
// no pane in particular and reaches the hub's first runtime.
func splitRuntimePath(p string) (id, path string) {
	if !strings.HasPrefix(p, runtimePrefix) {
		return "", p
	}
	rest := strings.TrimPrefix(p, runtimePrefix)
	id, path, found := strings.Cut(rest, "/")
	if !found {
		return id, "/"
	}
	return id, "/" + path
}

// isHubPath covers the routes that belong to the hub rather than to any one
// runtime: the pane list and the workspace tree the sidebar reads.
func isHubPath(p string) bool {
	p = strings.TrimSuffix(p, "/")
	return p == "/runtimes" || strings.HasPrefix(p, "/runtimes/") ||
		p == "/tree" || strings.HasPrefix(p, "/tree/")
}

func isAPIPath(p string) bool {
	p = strings.TrimSuffix(p, "/")
	if apiPaths[p] {
		return true
	}
	for _, prefix := range apiPrefixes {
		if strings.HasPrefix(p+"/", prefix) && p+"/" != prefix {
			return true
		}
	}
	return false
}

func main() {
	// A macOS install re-executes this binary to swap the bundle after the old
	// process exits. That child must never reach run(), or the swap would race a
	// second Studio booting on top of the directory it is replacing.
	if handled, code := update.MaybeRunMacHandoff(os.Args[1:]); handled {
		os.Exit(code)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "reasonix-next:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	bc := serve.NewBroadcaster()
	// A window opens where it was left, not where its shortcut happened to point.
	root := boot.ResolveWorkspaceRoot(lastWorkspace())
	built, err := boot.BuildRuntime(ctx, boot.Options{
		WorkspaceRoot: root,
		SessionDir:    serve.SessionDirFor(root),
		Sink:          bc,
		Stderr:        os.Stderr,
	})
	if err != nil {
		return err
	}
	ctrl := built.Controller
	// No EnsureSessionPath here: minting the file at launch left one empty
	// transcript behind every time the window opened. The first turn creates
	// it, and the inbox ensures its own path when it enqueues.

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Ask/Auto/YOLO is a posture the user set on the composer, not a per-launch
	// default — the old shell has read this since it had a picker.
	ctrl.SetToolApprovalMode(cfg.DesktopDefaultToolApprovalMode())
	shell := &App{pumps: map[string]context.CancelFunc{}}
	// One hub, several panes: each session gets its own runtime, so a second
	// conversation runs beside the first instead of rebuilding it.
	hub := serve.NewHub(serve.HubOptions{
		Serve:   cfg.Serve,
		Grant:   grantWindowCapabilities,
		OnOpen:  shell.startPump,
		OnClose: shell.stopPump,
	})
	shell.hub = hub
	srv := serve.New(ctrl, bc, cfg.Serve)
	srv.AdoptRuntime(built)
	// Without this a past save-conflict loop's forks stay on disk forever: the
	// CLI and the old shell each sweep them, this window is the third host.
	srv.StartRecoveryGC(ctx)
	hub.Adopt(srv, bc)
	api := hub.Handler()
	defer hub.Shutdown()

	assets, err := frontendAssets()
	if err != nil {
		return err
	}

	return wails.Run(&options.App{
		Title:  "Reasonix Studio",
		Width:  1440,
		Height: 900,
		// The top row is the title bar — project, goal, preset — so a native one
		// above it costs a whole row to say the app's own name. Same treatment
		// the existing shell uses; Linux keeps its frame, WebKitGTK has no inset.
		Frameless: goruntime.GOOS == "windows",
		Mac:       &mac.Options{TitleBar: mac.TitleBarHiddenInset(), Appearance: mac.DefaultAppearance},
		Windows:   &windows.Options{Theme: windows.SystemDefault},
		MinWidth:  760,
		MinHeight: 480,
		Menu:      appMenu(),
		// Production builds ship without one, so the window had no copy, paste,
		// or select-all on right-click — in a text editor that reads as broken.
		EnableDefaultContextMenu: true,
		Bind:                     []any{shell},
		DragAndDrop:              dragAndDrop(),
		OnStartup: func(ctx context.Context) {
			shell.mu.Lock()
			shell.ctx = ctx
			shell.mu.Unlock()
			applyDockIcon()
			// Panes opened before the window came up have no pump yet; from here
			// on the hub's OnOpen starts one as each is published.
			for _, rt := range hub.Runtimes() {
				shell.startPump(rt)
			}
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
			// In-process HTTP: no port, no CORS, no second transport to keep in
			// sync with the browser build.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					id, path := splitRuntimePath(r.URL.Path)
					// The bus has no reconnect handshake, so a reloaded page asks
					// for the replay resubscribing to /events would have given it.
					if path == replayPath {
						if rt := hub.Get(id); rt != nil {
							rt.Server.Controller().ReplayPendingPromptsWith(func() event.Sink { return rt.Events })
						}
						w.WriteHeader(http.StatusNoContent)
						return
					}
					if isAPIPath(path) || isHubPath(path) {
						api.ServeHTTP(w, r)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		OnShutdown: func(context.Context) { hub.Shutdown() },
	})
}

// appMenu is what makes ⌘C work: a WKWebView takes its editing shortcuts from
// the application's Edit menu, and a window without one has no copy, paste, or
// undo at all. macOS only — elsewhere the same bar renders as a stray
// in-window strip, and those platforms bind the shortcuts themselves.
func appMenu() *menu.Menu {
	if goruntime.GOOS != "darwin" {
		return nil
	}
	m := menu.NewMenu()
	m.Append(menu.AppMenu())
	m.Append(menu.EditMenu())
	m.Append(menu.WindowMenu())
	return m
}

// App is the one thing the SPA cannot do over HTTP: open the platform's folder
// picker. Everything else it needs is a route on the embedded hub. It also owns
// the per-runtime event pumps, because the bus is the window's, not a server's.
type App struct {
	hub *serve.Hub
	ctx context.Context

	mu    sync.Mutex
	pumps map[string]context.CancelFunc
}

// grantWindowCapabilities opens up what only a local window may do. The single
// client is the person in front of it, so the folder picker, the account token
// and provider keys are local dialogs rather than remote capabilities — and
// every pane gets them, not just the one the window started with.
func grantWindowCapabilities(srv *serve.Server) {
	srv.AllowWorkspaceSwitch()
	srv.AllowAccountAuth()
	srv.AllowProviderEdit()
	// Without this the first-run connection step never appears: /provider-setup
	// answers 404 while it is off, the window reads that as "nothing to set up",
	// and a machine with no key lands in the composer where every turn fails.
	srv.EnableProviderSetup()
}

// startPump forwards one runtime's frames onto the Wails bus under its own
// event name. Before OnStartup there is no context to emit into; the shell
// pumps whatever the hub already holds once the window comes up.
func (a *App) startPump(rt *serve.Runtime) {
	if rt == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil || a.pumps[rt.ID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.pumps[rt.ID] = cancel
	go pumpEvents(ctx, rt)
}

func (a *App) stopPump(rt *serve.Runtime) {
	if rt == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if cancel := a.pumps[rt.ID]; cancel != nil {
		cancel()
		delete(a.pumps, rt.ID)
	}
}

// currentRoot is the folder the picker should open at: whichever pane the hub
// lists first. A window with no pane left has nothing to anchor to.
func (a *App) currentRoot() string {
	for _, rt := range a.hub.Runtimes() {
		if root := rt.Server.Controller().WorkspaceRoot(); root != "" {
			return root
		}
	}
	return ""
}

// PickWorkspace returns the folder the user chose, or "" if they cancelled.
// Choosing does not switch anything — the SPA posts the path back to
// /workspace, so the browser build reaches the same code by typing one.
func (a *App) PickWorkspace() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	// The panel can make a folder — that is what CanCreateDirectories is for —
	// but nothing said so, and "打开" reads as "pick one that exists". Users
	// concluded the app could only open projects, never start one.
	opts := runtime.OpenDialogOptions{Title: "选择工作目录 · 也可以在这里新建一个", CanCreateDirectories: true}
	// Wails refuses to open the panel at all when this points at nothing, and
	// answers with an error instead — a workspace that has since been moved
	// would take the picker down with it.
	if root := a.currentRoot(); root != "" {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			opts.DefaultDirectory = root
		}
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, opts)
	if err != nil {
		slog.Warn("studio: folder picker", "err", err)
		return "", err
	}
	return dir, nil
}

// OpenExternal hands a link to the platform browser. A WKWebView answers a
// target="_blank" click with nothing at all — Wails binds no delegate for it —
// so every link in the window is dead until something routes it out. http(s)
// only: these come from model output, which may not reach the OS opener with a
// scheme of its choosing.
func (a *App) OpenExternal(rawURL string) {
	if a.ctx == nil {
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		slog.Warn("studio: refused to open a link", "url", rawURL)
		return
	}
	runtime.BrowserOpenURL(a.ctx, u.String())
}

// lastWorkspace is the folder this shell was driving when it last closed, or ""
// to let boot resolve one from the process working directory.
func lastWorkspace() string {
	for _, dir := range serve.Workspaces() {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}

// The asset server buffers a response until its handler returns, so GET /events
// never delivers a byte inside the shell — the SSE handler is still streaming
// when the page gives up. Wails' own bus is the one channel that does push, so
// the shell forwards the same broadcaster frames the browser reads over SSE.
// The payload is byte-identical either way; only the transport differs.
func pumpEvents(ctx context.Context, rt *serve.Runtime) {
	var ch <-chan []byte
	var unsubscribe func()
	// The same handoff GET /events performs: subscribe and replay as one step,
	// so a prompt already waiting for an answer survives the handover.
	rt.Server.Controller().ReplayPendingPromptsWith(func() event.Sink {
		ch, unsubscribe = rt.Events.Subscribe()
		return event.FuncSink(func(e event.Event) { rt.Events.EmitTo(ch, e) })
	})
	defer unsubscribe()
	name := runtimeEventName(rt.ID)
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			runtime.EventsEmit(ctx, name, string(data))
		case <-ctx.Done():
			return
		}
	}
}

// runtimeEventName is the bus channel one pane listens on. Panes run at the
// same time, so a single channel would interleave two conversations into both.
func runtimeEventName(id string) string { return wailsEventName + ":" + id }

func frontendAssets() (fs.FS, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{
		filepath.Join(filepath.Dir(exe), "frontend-next", "dist"),
		filepath.Join("..", "frontend-next", "dist"),
		filepath.Join("frontend-next", "dist"),
	} {
		if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
			return os.DirFS(dir), nil
		}
	}
	return nil, fmt.Errorf("frontend-next/dist not found: run `pnpm build` in desktop/frontend-next first")
}
