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
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/instanceid"
	"reasonix/internal/notify"
	"reasonix/internal/remotehost"
	"reasonix/internal/serve"
	"reasonix/internal/surface"
	"reasonix/internal/traystate"
	"reasonix/internal/update"

	// Kinds register from init, so a binary builds only what it links. Without
	// these every Anthropic model answers "unknown kind" at switch time.
	_ "reasonix/internal/provider/anthropic"
	_ "reasonix/internal/provider/openai"
	_ "reasonix/internal/provider/responses"
)

var version = "dev"

const (
	// credentialBytes is the width of the credential one launch is guarded by.
	credentialBytes = 32
	// handshakeVersion is the shape of the line on stdout. A parent that does
	// not recognise it must refuse the launch rather than read past it.
	handshakeVersion = 1
	// studioPrefix is the one path this host owns. Everything else is the
	// hub's, so a route added to the kernel later needs no change here.
	studioPrefix = "/_studio/"
)

// studioInstall is what the shell told us about itself. A launch that named no
// version gets no install at all, so the version routes refuse by name rather
// than answering for a build nobody claimed. The layout stays empty until a
// shell can state one: reading the catalog needs no install path, and applying
// one from a guess is how a swap lands on the wrong bundle.
func studioInstall(version string) *update.Install {
	if strings.TrimSpace(version) == "" {
		return nil
	}
	return &update.Install{Version: version}
}

func main() {
	page := flag.String("page", "", "directory holding the built Studio page")
	identity := flag.Bool("instance-id", false, "print the Studio instance this data home belongs to, and exit")
	// The shell around this process knows which build it is; this process does
	// not. os.Executable() here names the host binary, not the application.
	studioVersion := flag.String("studio-version", "", "the Studio build this host is serving")
	flag.Parse()
	// Which launches are the same Studio is Reasonix's question, not a shell's:
	// the answer is the canonicalized data home, and a shell asks for it rather
	// than working it out from an environment it does not resolve.
	if *identity {
		fmt.Fprintln(os.Stdout, instanceid.Current())
		return
	}
	os.Exit(run(parentLease(os.Stdin), os.Stdout, os.Stderr, *page, *studioVersion))
}

// run serves until the lease ends or the process is signalled. stdout carries
// the handshake and nothing else, because the parent parses the first line it
// reads there; every log goes to logs.
func run(lease io.Reader, handshakeTo, logs io.Writer, page, studioVersion string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if lease != nil {
		// The parent's end of the pipe closing is the parent going away, exit
		// or crash alike. Nothing else says that on all three platforms.
		go func() {
			_, _ = io.Copy(io.Discard, lease)
			stop()
		}()
	}

	served, err := studioPage(page, logs)
	if err != nil {
		fmt.Fprintln(logs, "reasonix-studio-host:", err)
		return 1
	}

	hub, err := assemble(ctx, logs, studioVersion)
	if err != nil {
		fmt.Fprintln(logs, "reasonix-studio-host:", err)
		return 1
	}
	// After the socket has drained, never before: a pane torn down under a live
	// request answers it with a half-closed kernel.
	defer hub.Shutdown()

	bound, err := bind(withStudioPage(hub.Handler(), served))
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
	Version int    `json:"version"`
	Origin  string `json:"origin"`
	Token   string `json:"token"`
}

func announce(w io.Writer, b *bound) error {
	return json.NewEncoder(w).Encode(handshake{Version: handshakeVersion, Origin: b.origin, Token: b.token})
}

// parentLease is the pipe a parent holds open for as long as it wants this host
// alive. Only a pipe is a lease: a terminal or /dev/null would either never end
// or end at once, and neither says anything about a parent.
func parentLease(f *os.File) io.Reader {
	st, err := f.Stat()
	if err != nil || st.Mode()&os.ModeNamedPipe == 0 {
		return nil
	}
	return f
}

// withStudioPage puts the built page under a namespace of its own and leaves
// every other path to the hub. The inverse — a list of the kernel's routes,
// with everything else falling through to the page — is what the asset server
// forced, and it has to be edited every time the kernel grows a route.
func withStudioPage(hub http.Handler, page fs.FS) http.Handler {
	if page == nil {
		return hub
	}
	files := http.StripPrefix(studioPrefix, http.FileServer(http.FS(page)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, studioPrefix) {
			hub.ServeHTTP(w, r)
			return
		}
		// A path inside the namespace that names no file is the page routing
		// itself, not a missing asset. Confined to the namespace, so it can
		// never answer for a route the kernel owns.
		name := strings.TrimPrefix(r.URL.Path, studioPrefix)
		if name != "" {
			if _, err := fs.Stat(page, strings.TrimSuffix(name, "/")); err != nil {
				http.ServeFileFS(w, r, page, "index.html")
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

// studioPage finds the built page. An explicit directory that holds none is a
// launch that would open on nothing, so it fails here rather than at the first
// paint; without one the host serves the kernel alone and says so.
func studioPage(dir string, logs io.Writer) (fs.FS, error) {
	if dir != "" {
		if !hasIndex(dir) {
			return nil, fmt.Errorf("no index.html under %s", dir)
		}
		return os.DirFS(dir), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	for _, c := range []string{
		filepath.Join(filepath.Dir(exe), "frontend-next", "dist"),
		filepath.Join("..", "frontend-next", "dist"),
		filepath.Join("desktop", "frontend-next", "dist"),
	} {
		if hasIndex(c) {
			return os.DirFS(c), nil
		}
	}
	fmt.Fprintln(logs, "reasonix-studio-host: no built page found; serving the kernel only")
	return nil, nil
}

func hasIndex(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !st.IsDir()
}

// assemble builds the hub this host serves: one pane on the workspace it was
// launched in, carrying the capabilities a local window may exercise.
func assemble(ctx context.Context, logs io.Writer, studioVersion string) (*serve.Hub, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	// This window is the only client of its kernel, so a system notification
	// reaches the person who asked for it. Every pane gets the same wrapper,
	// not just the one the launch started with.
	notifications := hostNotifications(cfg)
	// One fold behind the status icon: every pane's events on the way through,
	// so the tray surface answers from what the panes did rather than from a
	// count the shell kept for itself.
	tracker := traystate.New(nil)
	decorate := func(sink event.Sink) event.Sink {
		return tracker.Watch(paneKey(sink), notifications(sink))
	}
	bc := serve.NewBroadcaster()
	paneSink := decorate(bc)
	root := boot.ResolveWorkspaceRoot("")
	built, err := boot.BuildRuntime(ctx, boot.Options{
		Version:       version,
		WorkspaceRoot: root,
		SessionDir:    serve.SessionDirFor(root),
		Sink:          paneSink,
		Stderr:        logs,
		StatsSource:   surface.Desktop,
	})
	if err != nil {
		return nil, err
	}
	// A first connect can stop for a host key nobody has seen or a locked key.
	// Both are questions, and the broker is where they live until answered.
	asks := serve.NewAskBroker(nil)
	hubCfg := hostServeConfig(cfg.Serve)
	hub := serve.NewHub(serve.HubOptions{
		Serve:        hubCfg,
		Surface:      surface.Desktop,
		Grant:        grantHostCapabilities,
		DecorateSink: decorate,
		Tray:         &studioTray{tracker: tracker},
		Asks:         asks,
		Remote:       remotehost.New(ctx, version, asks),
		OnClose:      func(rt *serve.Runtime) { tracker.Drop(paneKey(rt.Events)) },
		Install:      studioInstall(studioVersion),
	})
	srv := serve.New(built.Controller, bc, hubCfg)
	srv.SetPaneSink(paneSink)
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

// hostNotifications is the sink wrapper every runtime gets. Off unless the
// shared [notifications] config asks for it, so the CLI and this window answer
// to one setting rather than each growing its own.
func hostNotifications(cfg *config.Config) func(event.Sink) event.Sink {
	if cfg == nil || !cfg.Notifications.Enabled {
		return func(sink event.Sink) event.Sink { return sink }
	}
	sender := notify.NewPlatformSender()
	return func(sink event.Sink) event.Sink { return notify.NewSink(sink, sender, cfg.Notifications) }
}

// studioTray answers for a shell that can put an icon back whenever the setting
// asks for one: what is set is what is up, so the two never drift the way they
// do on a platform that gives its icon up once per process.
type studioTray struct{ tracker *traystate.Tracker }

func (t *studioTray) IconLive() bool {
	return config.LoadForEdit(config.UserConfigPath()).DesktopTray() != "off"
}

func (t *studioTray) TrayFold() traystate.State { return t.tracker.State() }

// The window reads what it asked for out of the answer, so there is nothing to
// push at it here.
func (t *studioTray) ApplyTrayPrefs(serve.TrayPrefs) {}

// paneKey names a pane by the sink it emits through: the hub hands that to the
// decorator before the pane has an id, and hands the same one back on close.
func paneKey(sink any) string { return fmt.Sprintf("%p", sink) }

// grantHostCapabilities opens what only a local window may do. The single
// client is the person in front of it, so provider keys and the account token
// are local decisions rather than capabilities reachable from a network.
func grantHostCapabilities(srv *serve.Server) {
	srv.AllowWorkspaceSwitch()
	srv.AllowAccountAuth()
	srv.AllowProviderEdit()
}
