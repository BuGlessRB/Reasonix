package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/serve"
	"reasonix/internal/surface"
	"reasonix/internal/testenv"
)

func TestMain(m *testing.M) {
	testenv.RunWithIsolatedUserState(m)
}

// recordingRunner stands in for the agent: a turn that reaches it proves the
// request crossed the boundary and landed in the kernel, which is the only
// thing a status code cannot say.
type recordingRunner struct{ got chan string }

func (r recordingRunner) Run(_ context.Context, input string) error {
	r.got <- input
	return nil
}

// testHub is the production assembly with one piece swapped: the runner. The
// hub, its config path and its capabilities are the ones the host builds.
func testHub(t *testing.T, cfg config.ServeConfig) (*serve.Hub, chan string) {
	t.Helper()
	root := t.TempDir()
	got := make(chan string, 1)
	bc := serve.NewBroadcaster()
	ctrl := control.New(control.Options{
		Runner:        recordingRunner{got: got},
		Sink:          bc,
		WorkspaceRoot: root,
		SessionDir:    serve.SessionDirFor(root),
	})
	t.Cleanup(ctrl.Close)
	hubCfg := hostServeConfig(cfg)
	hub := serve.NewHub(serve.HubOptions{Serve: hubCfg, Surface: surface.Desktop, Grant: grantHostCapabilities})
	srv := serve.New(ctrl, bc, hubCfg)
	srv.SetPaneSink(bc)
	if _, err := hub.Adopt(srv, bc); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	t.Cleanup(hub.Shutdown)
	return hub, got
}

// startHost binds and serves the way run does, and reports when the socket has
// finished draining so a test can assert on what is left behind.
func startHost(t *testing.T, cfg config.ServeConfig) (*bound, chan string, func()) {
	t.Helper()
	hub, got := testHub(t, cfg)
	b, err := bind(hub.Handler())
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.serve(ctx) }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve returned %v, want a clean drain", err)
		}
	}
	t.Cleanup(stop)
	return b, got, stop
}

// ask sends a request the host's own page would send, unless a test overrides
// one of the pieces the boundary reads.
func ask(t *testing.T, b *bound, method, path string, mut func(*http.Request)) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, b.origin+path, strings.NewReader(`{"input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", b.origin)
	req.AddCookie(&http.Cookie{Name: serve.TokenCookie, Value: b.token})
	if mut != nil {
		mut(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	return resp.StatusCode, string(body[:n])
}

// The socket is never reachable from anywhere but this machine, and the address
// is what the origin and every later check are derived from.
func TestHostBindsOnlyToLoopback(t *testing.T) {
	b, err := bind(http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer b.listener.Close()

	host, port, err := net.SplitHostPort(b.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Errorf("listening on %q, want 127.0.0.1", host)
	}
	if want := "http://" + net.JoinHostPort(host, port); b.origin != want {
		t.Errorf("origin = %q, want %q", b.origin, want)
	}
}

// The credential belongs to one launch. Two hosts started from the same machine
// and the same config must not be able to drive each other.
func TestHostMintsAFreshCredentialPerLaunch(t *testing.T) {
	first, err := bind(http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer first.listener.Close()
	second, err := bind(http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer second.listener.Close()

	if first.token == second.token {
		t.Fatal("two launches minted the same credential")
	}
	if len(first.token) != credentialBytes*2 {
		t.Errorf("credential is %d hex chars, want %d", len(first.token), credentialBytes*2)
	}
}

// Studio's boundary is not the user's serve policy. A configuration that turns
// authentication off for `reasonix serve` does not turn it off here.
func TestHostRequiresItsCredentialUnderAuthModeNone(t *testing.T) {
	b, got, _ := startHost(t, config.ServeConfig{AuthMode: "none"})

	status, body := ask(t, b, http.MethodPost, "/submit", func(r *http.Request) {
		r.Header.Del("Cookie")
	})
	if status != http.StatusForbidden || !strings.Contains(body, "loopback.unauthorized") {
		t.Fatalf("status = %d %s, want 403 loopback.unauthorized", status, body)
	}
	select {
	case in := <-got:
		t.Fatalf("an unauthenticated request reached the kernel with %q", in)
	default:
	}
}

// A token the user configured for `reasonix serve` is not this launch's
// credential, and it does not become one by being in the config file.
func TestHostDoesNotAdoptTheConfiguredServeToken(t *testing.T) {
	const configured = "a-token-the-user-put-in-their-config"
	b, got, _ := startHost(t, config.ServeConfig{AuthMode: "token", Token: configured})

	if b.token == configured {
		t.Fatal("the host adopted the configured serve token as its own credential")
	}
	status, body := ask(t, b, http.MethodPost, "/submit", func(r *http.Request) {
		r.Header.Del("Cookie")
		r.AddCookie(&http.Cookie{Name: serve.TokenCookie, Value: configured})
	})
	if status != http.StatusForbidden || !strings.Contains(body, "loopback.unauthorized") {
		t.Fatalf("status = %d %s, want 403 loopback.unauthorized", status, body)
	}
	select {
	case in := <-got:
		t.Fatalf("the configured token drove the kernel with %q", in)
	default:
	}

	// And the launch credential still works, which is what proves the two gates
	// are not both trying to own the one cookie.
	if status, body := ask(t, b, http.MethodPost, "/submit", nil); status >= http.StatusBadRequest {
		t.Fatalf("status = %d %s, want the turn accepted", status, body)
	}
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never reached the kernel")
	}
}

// Reads and writes both have to arrive, or the boundary is only proving it can
// refuse things.
func TestHostPassesRealTrafficToTheKernel(t *testing.T) {
	b, got, _ := startHost(t, config.ServeConfig{})

	status, body := ask(t, b, http.MethodGet, "/status", func(r *http.Request) {
		r.Header.Del("Origin")
	})
	if status != http.StatusOK {
		t.Fatalf("GET /status = %d %s, want 200", status, body)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("GET /status did not answer with the kernel's JSON: %v", err)
	}

	if status, body := ask(t, b, http.MethodPost, "/submit", nil); status >= http.StatusBadRequest {
		t.Fatalf("POST /submit = %d %s", status, body)
	}
	select {
	case in := <-got:
		if in != "hello" {
			t.Errorf("the kernel ran %q, want %q", in, "hello")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never reached the kernel")
	}
}

// The stream still streams. A gate that buffered would leave every frame
// waiting for the turn to end, which is the failure Wails' asset server had.
func TestHostStreamsEventsThroughTheGate(t *testing.T) {
	b, _, _ := startHost(t, config.ServeConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.origin+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: serve.TokenCookie, Value: b.token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/events = %d, want 200", resp.StatusCode)
	}
	// The opening comment arrives before any event does, and only because the
	// handler flushed it — nothing else has been written yet.
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("the stream delivered nothing: %v", err)
	}
	if !strings.HasPrefix(line, ":") {
		t.Fatalf("first line = %q, want the stream's opening comment", line)
	}
}

// Draining has to actually release the socket: a host that returned while the
// port was still bound would refuse the next launch its own listener.
func TestHostReleasesTheSocketOnShutdown(t *testing.T) {
	b, _, stop := startHost(t, config.ServeConfig{})
	addr := b.listener.Addr().String()

	if status, body := ask(t, b, http.MethodGet, "/status", nil); status != http.StatusOK {
		t.Fatalf("GET /status = %d %s, want 200 before shutdown", status, body)
	}
	stop()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err == nil {
		conn.Close()
		t.Fatalf("%s still accepts connections after shutdown", addr)
	}
}
