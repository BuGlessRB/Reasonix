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
	"reasonix/internal/control"
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
	ctrl, err := boot.Build(ctx, boot.Options{Sink: bc, Stderr: os.Stderr})
	if err != nil {
		return err
	}
	defer ctrl.Close()
	// No EnsureSessionPath here: minting the file at launch left one empty
	// transcript behind every time the window opened. The first turn creates
	// it, and the inbox ensures its own path when it enqueues.

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	api := serve.New(ctrl, bc, cfg.Serve).Handler()

	assets, err := frontendAssets()
	if err != nil {
		return err
	}

	return wails.Run(&options.App{
		Title:     "Reasonix Studio",
		Width:     1440,
		Height:    900,
		OnStartup: func(ctx context.Context) { go pumpEvents(ctx, ctrl, bc) },
		AssetServer: &assetserver.Options{
			Assets: assets,
			// In-process HTTP: no port, no CORS, no second transport to keep in
			// sync with the browser build.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// The bus has no reconnect handshake, so a reloaded page asks
					// for the replay resubscribing to /events would have given it.
					if r.URL.Path == replayPath {
						ctrl.ReplayPendingPromptsWith(func() event.Sink { return bc })
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
		OnShutdown: func(context.Context) { ctrl.Close() },
	})
}

// The asset server buffers a response until its handler returns, so GET /events
// never delivers a byte inside the shell — the SSE handler is still streaming
// when the page gives up. Wails' own bus is the one channel that does push, so
// the shell forwards the same broadcaster frames the browser reads over SSE.
// The payload is byte-identical either way; only the transport differs.
func pumpEvents(ctx context.Context, ctrl *control.Controller, bc *serve.Broadcaster) {
	var ch <-chan []byte
	var unsubscribe func()
	// The same handoff GET /events performs: subscribe and replay as one step,
	// so a prompt already waiting for an answer survives the handover.
	ctrl.ReplayPendingPromptsWith(func() event.Sink {
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
