// Command reasonix-studio-host runs Studio's kernel behind a loopback socket.
// It is the half of the shell that is not a window: the hub, its panes and its
// event streams over real HTTP, guarded by a boundary this process owns, so a
// renderer living in another process reaches the same surface a browser does.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/serve"
	"reasonix/internal/surface"

	// Kinds register from init, so a binary builds only what it links. Without
	// these every Anthropic model answers "unknown kind" at switch time.
	_ "reasonix/internal/provider/anthropic"
	_ "reasonix/internal/provider/openai"
	_ "reasonix/internal/provider/responses"
)

var version = "dev"

// credentialBytes is the width of the credential one launch is guarded by.
const credentialBytes = 32

func main() {
	os.Exit(run(os.Stdout, os.Stderr))
}

func run(handshakeTo io.Writer, logs io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hub, err := assemble(ctx, logs)
	if err != nil {
		fmt.Fprintln(logs, "reasonix-studio-host:", err)
		return 1
	}
	// After the socket has drained, never before: a pane torn down under a live
	// request answers it with a half-closed kernel.
	defer hub.Shutdown()

	bound, err := bind(hub.Handler())
	if err != nil {
		fmt.Fprintln(logs, "reasonix-studio-host:", err)
		return 1
	}
	// The credential-writing setup surface opens only on a loopback address,
	// and this is the first host that has one to show it.
	hub.EnableProviderSetupForListener(bound.listener.Addr().String())
	if err := announce(handshakeTo, bound); err != nil {
		fmt.Fprintln(logs, "reasonix-studio-host:", err)
		return 1
	}
	if err := bound.serve(ctx); err != nil {
		fmt.Fprintln(logs, "reasonix-studio-host:", err)
		return 1
	}
	return 0
}

// bound is a hub on a socket: the listener, and the guarded handler that is the
// only way through it.
type bound struct {
	listener net.Listener
	origin   string
	token    string
	handler  http.Handler
}

// bind is the startup order the boundary depends on. The socket comes first
// because nothing can name the origin until the kernel owns a port, and the
// credential is minted here rather than read, so no configuration reaches it.
func bind(next http.Handler) (*bound, error) {
	ln, err := serve.ListenLoopback()
	if err != nil {
		return nil, err
	}
	token, err := launchCredential()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	origin := serve.LoopbackOrigin(ln)
	return &bound{
		listener: ln,
		origin:   origin,
		token:    token,
		handler:  serve.NewLoopbackGate(next, serve.LoopbackGateOptions{Token: token, Origin: origin}),
	}, nil
}

// serve runs until ctx ends, then drains. The gate sits outside the hub, so
// nothing here changes what the hub's own auth and CSRF middleware do.
func (b *bound) serve(ctx context.Context) error {
	return serve.RunGracefulHandler(ctx, b.listener, b.handler)
}

// launchCredential mints what this launch is guarded by. Never read from
// configuration and never persisted: a credential a user can set is one that a
// page which can read their config can present.
func launchCredential() (string, error) {
	buf := make([]byte, credentialBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// handshake is what the parent process needs and nothing else. It goes down the
// pipe the parent already holds; a file or an environment variable would outlive
// the launch and be readable by more than the one process that spawned it.
type handshake struct {
	Origin string `json:"origin"`
	Token  string `json:"token"`
}

func announce(w io.Writer, b *bound) error {
	return json.NewEncoder(w).Encode(handshake{Origin: b.origin, Token: b.token})
}

// assemble builds the hub this host serves: one pane on the workspace it was
// launched in, carrying the capabilities a local window may exercise.
func assemble(ctx context.Context, logs io.Writer) (*serve.Hub, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	bc := serve.NewBroadcaster()
	root := boot.ResolveWorkspaceRoot("")
	built, err := boot.BuildRuntime(ctx, boot.Options{
		Version:       version,
		WorkspaceRoot: root,
		SessionDir:    serve.SessionDirFor(root),
		Sink:          bc,
		Stderr:        logs,
		StatsSource:   surface.Desktop,
	})
	if err != nil {
		return nil, err
	}
	hubCfg := hostServeConfig(cfg.Serve)
	hub := serve.NewHub(serve.HubOptions{
		Serve:   hubCfg,
		Surface: surface.Desktop,
		Grant:   grantHostCapabilities,
	})
	srv := serve.New(built.Controller, bc, hubCfg)
	srv.SetPaneSink(bc)
	srv.AdoptRuntime(built)
	if _, err := hub.Adopt(srv, bc); err != nil {
		hub.Shutdown()
		return nil, err
	}
	hub.StartRecoveryGC(ctx)
	return hub, nil
}

// hostServeConfig is the user's serve settings with their authentication taken
// out. Studio's boundary is the loopback gate, and both gates read one cookie:
// a configured token left in place would have the hub refuse the credential
// this launch minted, on every request. Forwarded headers go with it — the
// boundary decides on the address it was reached at, not on a claim about it.
func hostServeConfig(cfg config.ServeConfig) config.ServeConfig {
	cfg.AuthMode = ""
	cfg.Token = ""
	cfg.PasswordHash = ""
	cfg.BehindProxy = false
	return cfg
}

// grantHostCapabilities opens what only a local window may do. The single
// client is the person in front of it, so provider keys and the account token
// are local decisions rather than capabilities reachable from a network.
func grantHostCapabilities(srv *serve.Server) {
	srv.AllowWorkspaceSwitch()
	srv.AllowAccountAuth()
	srv.AllowProviderEdit()
}
