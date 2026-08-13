// Command reasonix-next is the Wails shell for frontend-next. It differs from
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

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/serve"
)

// Everything the SPA is allowed to route to the kernel. Anything else falls
// through to the static assets, so an unknown path renders index.html rather
// than reaching the controller.
var apiPaths = map[string]bool{
	"/events": true, "/history": true, "/context": true, "/status": true,
	"/sessions": true, "/checkpoints": true, "/branches": true, "/models": true,
	"/submit": true, "/cancel": true, "/approve": true, "/answer": true,
	"/plan": true, "/preset": true, "/model": true, "/effort": true,
	"/goal": true, "/resume": true, "/compact": true, "/new": true,
	"/rewind": true, "/fork": true, "/summarize": true, "/forget": true,
	"/tool-approval-mode": true, "/auto-approve-tools": true, "/bypass": true,
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
	ctrl.EnsureSessionPath()

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
		Title:  "Reasonix",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// In-process HTTP: no port, no CORS, no second transport to keep in
			// sync with the browser build.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
