// Command reasonix-studio is the Wails shell for frontend-next. It differs from
// the existing desktop binary in one way that matters: the UI talks to the
// kernel over internal/serve's HTTP surface instead of Wails bindings, so the
// same build runs in a browser against `reasonix serve`.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/serve"
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
	"/provider-setup": true, "/delete-session": true, "/inbox/items": true,
	"/trajectory": true, "/slash": true, "/workspace": true, "/workspaces": true,
	"/mcp": true,
}

func main() {
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
	ctrl, err := boot.Build(ctx, boot.Options{
		WorkspaceRoot: root,
		SessionDir:    serve.SessionDirFor(root),
		Sink:          bc,
		Stderr:        os.Stderr,
	})
	if err != nil {
		return err
	}
	// No EnsureSessionPath here: minting the file at launch left one empty
	// transcript behind every time the window opened. The first turn creates
	// it, and the inbox ensures its own path when it enqueues.

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv := serve.New(ctrl, bc, cfg.Serve)
	// The only client is the window in front of the user, so the folder switch
	// this grants is a local file dialog rather than a remote capability.
	srv.AllowWorkspaceSwitch()
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
		Bind:   []any{shell},
		OnStartup: func(ctx context.Context) {
			shell.ctx = ctx
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
					if apiPaths[strings.TrimSuffix(r.URL.Path, "/")] {
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
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "打开工作目录",
		DefaultDirectory:     a.srv.Controller().WorkspaceRoot(),
		CanCreateDirectories: true,
	})
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
