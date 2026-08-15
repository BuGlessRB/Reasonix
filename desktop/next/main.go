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
	"/memory": true, "/network": true, "/todos": true, "/providers": true,
	"/changes": true, "/attachments": true, "/roles": true,
	"/themes": true, "/extensions": true, "/plugins": true, "/surfaces": true,
	"/welcome": true,
}

// A sub-path belongs to the resource it hangs off: /mcp/reconnect is the same
// surface as /mcp. Listing families rather than every leaf is what keeps a new
// endpoint from silently answering with index.html instead of JSON — and
// TestEveryPathTheFrontendCallsIsRouted is what keeps this list honest, because
// the comment alone did not.
var apiPrefixes = []string{"/mcp/", "/skills/", "/inbox/", "/account/", "/hooks/", "/memory/", "/network/", "/providers/", "/rewind/", "/extensions/", "/themes/", "/plugins/"}

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
	srv := serve.New(ctrl, bc, cfg.Serve)
	srv.AdoptRuntime(built)
	// The only client is the window in front of the user, so the folder switch
	// this grants is a local file dialog rather than a remote capability.
	srv.AllowWorkspaceSwitch()
	// Same reasoning as the folder switch: the only client is this window, and
	// the token lands in this machine's own credential store.
	srv.AllowAccountAuth()
	// And the same again for provider keys, which land in the same store.
	srv.AllowProviderEdit()
	// Without this the first-run connection step never appears: /provider-setup
	// answers 404 while it is off, the window reads that as "nothing to set up",
	// and a machine with no key lands in the composer where every turn fails.
	srv.EnableProviderSetup()
	api := srv.Handler()
	// Read the controller through the server from here on: a model, extension,
	// or workspace switch replaces it, and the one built above is then dead.
	defer func() { srv.Controller().Close() }()
	shell := &App{srv: srv}

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
			shell.ctx = ctx
			applyDockIcon()
			go pumpEvents(ctx, srv, bc)
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
			// In-process HTTP: no port, no CORS, no second transport to keep in
			// sync with the browser build.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// The bus has no reconnect handshake, so a reloaded page asks
					// for the replay resubscribing to /events would have given it.
					if r.URL.Path == replayPath {
						srv.Controller().ReplayPendingPromptsWith(func() event.Sink { return bc })
						w.WriteHeader(http.StatusNoContent)
						return
					}
					if isAPIPath(r.URL.Path) {
						api.ServeHTTP(w, r)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		OnShutdown: func(context.Context) { srv.Controller().Close() },
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
// picker. Everything else it needs is a route on the embedded server.
type App struct {
	srv *serve.Server
	ctx context.Context
}

// PickWorkspace returns the folder the user chose, or "" if they cancelled.
// Choosing does not switch anything — the SPA posts the path back to
// /workspace, so the browser build reaches the same code by typing one.
func (a *App) PickWorkspace() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	opts := runtime.OpenDialogOptions{Title: "打开工作目录", CanCreateDirectories: true}
	// Wails refuses to open the panel at all when this points at nothing, and
	// answers with an error instead — a workspace that has since been moved
	// would take the picker down with it.
	if root := a.srv.Controller().WorkspaceRoot(); root != "" {
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
func pumpEvents(ctx context.Context, srv *serve.Server, bc *serve.Broadcaster) {
	var ch <-chan []byte
	var unsubscribe func()
	// The same handoff GET /events performs: subscribe and replay as one step,
	// so a prompt already waiting for an answer survives the handover.
	srv.Controller().ReplayPendingPromptsWith(func() event.Sink {
		ch, unsubscribe = bc.Subscribe()
		return event.FuncSink(func(e event.Event) { bc.EmitTo(ch, e) })
	})
	defer unsubscribe()
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			runtime.EventsEmit(ctx, wailsEventName, string(data))
		case <-ctx.Done():
			return
		}
	}
}

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
