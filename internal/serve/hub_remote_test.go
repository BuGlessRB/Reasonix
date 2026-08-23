package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
)

// stubAttacher stands in for the window's link layer.
type stubAttacher struct {
	attach     func(host, workspace string) (RemoteEndpoint, func(), error)
	states     map[string]RemoteLinkState
	candidates []string
}

func (s *stubAttacher) Attach(_ context.Context, host, workspace string) (RemoteEndpoint, func(), error) {
	return s.attach(host, workspace)
}

func (s *stubAttacher) States() map[string]RemoteLinkState { return s.states }

func (s *stubAttacher) Candidates() []string { return s.candidates }

// remoteKernel stands in for a `reasonix serve` on another machine. It records
// what the proxy presented so the tests can assert on the far side.
type remoteKernel struct {
	*httptest.Server
	cookies chan string
	release chan struct{}
}

func fakeRemoteKernel(t *testing.T) *remoteKernel {
	t.Helper()
	rk := &remoteKernel{cookies: make(chan string, 8), release: make(chan struct{})}
	rk.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case rk.cookies <- r.Header.Get("Cookie"):
		default:
		}
		if r.URL.Path != "/events" {
			writeJSON(w, map[string]string{"path": r.URL.Path})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		fmt.Fprint(w, "data: first\n\n")
		flush.Flush()
		// The second frame is withheld until the test has the first in hand: a
		// proxy that buffers cannot pass this, and one that flushes cannot fail
		// it for a timing reason.
		<-rk.release
		fmt.Fprint(w, "data: second\n\n")
		flush.Flush()
	}))
	t.Cleanup(rk.Close)
	return rk
}

func remotePane(t *testing.T, h *Hub, addr string) *Runtime {
	t.Helper()
	rt, err := h.OpenRemote(RemoteEndpoint{
		Host:      "gpu-box",
		Workspace: "/srv/data/training",
		Addr:      addr,
		Token:     "remote-secret",
	}, nil)
	if err != nil {
		t.Fatalf("open remote: %v", err)
	}
	return rt
}

// A turn's output reaches the window one frame at a time. Buffering the stream
// anywhere between the two kernels turns a live transcript into a page that
// arrives when the turn is already over.
func TestRemotePaneStreamsEventsUnbuffered(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{})
	rt := remotePane(t, h, rk.Listener.Addr().String())
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, front.URL+rt.view().Base+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// One frame is two lines — the data and the blank one that ends it — so this
	// reads until the payloads arrive rather than counting lines.
	frames := make(chan string, 2)
	go func() {
		defer close(frames)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if payload, ok := strings.CutPrefix(line, "data:"); ok {
				frames <- strings.TrimSpace(payload)
			}
		}
	}()

	select {
	case got := <-frames:
		if got != "first" {
			t.Fatalf("first frame = %q, want %q", got, "first")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first frame never arrived: the proxy is buffering the stream")
	}
	close(rk.release)
	select {
	case got := <-frames:
		if got != "second" {
			t.Fatalf("second frame = %q, want %q", got, "second")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second frame never arrived")
	}
}

// The remote gate only knows its own token, and the window's cookies are for
// this machine. Forwarding the window's would authenticate nothing.
func TestRemotePaneCarriesTheRemoteToken(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{})
	rt := remotePane(t, h, rk.Listener.Addr().String())
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, front.URL+rt.view().Base+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: cookieToken, Value: "this-window's-token"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	select {
	case got := <-rk.cookies:
		if want := cookieToken + "=remote-secret"; got != want {
			t.Fatalf("remote saw Cookie %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the remote kernel was never reached")
	}
}

// A dead link and a refusing kernel are different problems with different
// fixes, and only a code can tell the window which one it has.
func TestRemotePaneRefusesWithACodeWhenTheLinkIsDown(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	addr := rk.Listener.Addr().String()
	rk.Close()

	h := NewHub(HubOptions{})
	rt := remotePane(t, h, addr)
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	resp, err := http.Get(front.URL + rt.view().Base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
	var body struct {
		Code   string            `json:"code"`
		Params map[string]string `json:"params"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "remote.unreachable" {
		t.Fatalf("code = %q, want %q", body.Code, "remote.unreachable")
	}
	if body.Params["host"] != "gpu-box" {
		t.Fatalf("params.host = %q, want %q", body.Params["host"], "gpu-box")
	}
}

// The pane list is what the sidebar and the tab strip label themselves from.
// A remote workspace is spelled by the remote OS, so its name is a slash path
// no matter which separator this machine uses.
func TestRemotePaneViewNamesItsHostAndWorkspace(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{})
	rt := remotePane(t, h, rk.Listener.Addr().String())

	view := rt.view()
	if view.Host != "gpu-box" {
		t.Fatalf("host = %q, want %q", view.Host, "gpu-box")
	}
	if view.Name != "training" {
		t.Fatalf("name = %q, want %q", view.Name, "training")
	}
	if view.Root != "/srv/data/training" {
		t.Fatalf("root = %q, want %q", view.Root, "/srv/data/training")
	}
	if rt.Local() {
		t.Fatal("a proxied pane reported itself as local")
	}
}

// Everything the host applies per pane reaches into a local assembly. A remote
// pane has none, so a window holding one must not crash on the sweep that
// walks them all.
func TestHostDecisionsSkipRemotePanes(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{})
	local := hubRuntime(t, h, t.TempDir())
	remotePane(t, h, rk.Listener.Addr().String())

	h.StartRecoveryGC(context.Background())
	h.EnableProviderSetupForListener("127.0.0.1:0")
	if got := h.rootPanes(local.Server.Controller().WorkspaceRoot()); got != 1 {
		t.Fatalf("rootPanes = %d, want 1", got)
	}
	if len(h.roots()) == 0 {
		t.Fatal("the workspace tree lost its local root")
	}
	if n := len(h.List()); n != 2 {
		t.Fatalf("hub lists %d panes, want 2", n)
	}
}

// Closing a proxied pane has nothing local to persist, but the connection it
// rode is shared: the hold has to come off, or the last pane never frees it.
func TestClosingARemotePaneReleasesItsHold(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{})
	released := make(chan struct{})
	rt, err := h.OpenRemote(RemoteEndpoint{
		Host:      "gpu-box",
		Workspace: "/srv/data/training",
		Addr:      rk.Listener.Addr().String(),
		Token:     "remote-secret",
	}, func() { close(released) })
	if err != nil {
		t.Fatalf("open remote: %v", err)
	}
	if err := h.Close(rt.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("closing the pane never released its hold on the connection")
	}
}

// A listening server must not dial onward because a request asked it to: that
// turns the kernel into someone else's route into the network behind it.
func TestOpenRemoteIsRefusedWhereNoAttacherIsWired(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	h := NewHub(HubOptions{})
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	resp, err := http.Post(front.URL+"/remotes/open", "application/json",
		strings.NewReader(`{"host":"gpu-box","workspace":"/srv/data"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotImplemented)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "remote.not_available" {
		t.Fatalf("code = %q, want %q", body.Code, "remote.not_available")
	}
}

// The endpoint is where a host's attach becomes a pane. What it returns is what
// the sidebar labels, so the host name has to survive the round trip.
func TestOpenRemoteEndpointPublishesTheProxiedPane(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{Remote: &stubAttacher{
		attach: func(host, workspace string) (RemoteEndpoint, func(), error) {
			return RemoteEndpoint{
				Host:      host,
				Workspace: workspace,
				Addr:      rk.Listener.Addr().String(),
				Token:     "remote-secret",
			}, func() {}, nil
		},
	}})
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	resp, err := http.Post(front.URL+"/remotes/open", "application/json",
		strings.NewReader(`{"host":"gpu-box","workspace":"/srv/data/training"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var view RuntimeView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Host != "gpu-box" || view.Name != "training" {
		t.Fatalf("view = %+v, want the remote host and workspace", view)
	}
	if rt := h.Get(view.ID); rt == nil || rt.Local() {
		t.Fatal("the endpoint published a local pane")
	}
}

// A hold taken by the attach and refused by the hub is a connection nobody will
// ever close: the pane it was for does not exist to be closed.
func TestOpenRemoteGivesBackTheHoldWhenTheHubRefuses(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	released := make(chan struct{}, 1)
	h := NewHub(HubOptions{Remote: &stubAttacher{
		attach: func(string, string) (RemoteEndpoint, func(), error) {
			// No workspace: the hub validates the endpoint and turns it down.
			return RemoteEndpoint{Host: "gpu-box", Addr: rk.Listener.Addr().String()},
				func() { released <- struct{}{} }, nil
		},
	}})
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	resp, err := http.Post(front.URL+"/remotes/open", "application/json", strings.NewReader(`{"host":"gpu-box"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("an endpoint with no workspace was accepted")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("the refused pane never gave its connection hold back")
	}
}

// The host book is configuration; what each link is doing is the attacher's.
// The row the sidebar draws needs both, and a host nobody has connected to
// still has to appear — that is where the connect button lives.
func TestRemoteHostsJoinTheBookWithLiveLinkState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	// Remote hosts are user-global, so the book lives in the home config, never
	// in a project file — the same rule `reasonix remote add` writes under.
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(`
[[remote.hosts]]
name = "gpu-box"
host = "10.0.0.4"
user = "ada"
port = 2222
workspace = "/srv/training"

[[remote.hosts]]
name = "spare"
host = "10.0.0.5"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHub(HubOptions{Remote: &stubAttacher{states: map[string]RemoteLinkState{
		"gpu-box": {Status: "degraded", Err: "forward 8080 未挂上", Panes: 2},
	}}})
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	var rows []RemoteHostView
	resp, err := http.Get(front.URL + "/remotes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("listed %d hosts, want 2", len(rows))
	}
	if rows[0].Target != "ada@10.0.0.4:2222" {
		t.Fatalf("target = %q, want the address a user would type", rows[0].Target)
	}
	if rows[0].Status != "degraded" || rows[0].Panes != 2 {
		t.Fatalf("live state was dropped: %+v", rows[0])
	}
	// A host with no link is idle, not missing: its row is where connecting starts.
	if rows[1].Name != "spare" || rows[1].Status != "idle" {
		t.Fatalf("unconnected host = %+v, want an idle row", rows[1])
	}
	if rows[1].Target != "10.0.0.5" {
		t.Fatalf("target = %q, want no user or default port spelled out", rows[1].Target)
	}
}

// The proxy strips the pane's prefix, so what it presents to the far hub has
// to name a runtime there. Without one it lands on that hub's default — and
// two panes on one workspace would drive a single conversation between them.
func TestTwoRemotePanesDriveTwoRuntimesOnTheFarSide(t *testing.T) {
	// The far hub builds a real assembly for each pane, so it needs a source
	// to build against — the same minimum the other Open-driven tests use.
	writeOpenableConfig(t)
	far := NewHub(HubOptions{})
	defer far.Shutdown()
	farSide := httptest.NewServer(far.Handler())
	defer farSide.Close()

	near := NewHub(HubOptions{Remote: &stubAttacher{
		attach: func(host, workspace string) (RemoteEndpoint, func(), error) {
			// One workspace, so both panes share an address: the whole point is
			// that sharing the link must not mean sharing the conversation.
			return RemoteEndpoint{
				Host: host, Workspace: workspace,
				Addr: farSide.Listener.Addr().String(), Token: "t",
			}, func() {}, nil
		},
	}})
	nearSide := httptest.NewServer(near.Handler())
	defer nearSide.Close()

	// One workspace for both, which is the case that was broken: they share a
	// link and an address, and must still be two conversations.
	shared := t.TempDir()
	open := func() RuntimeView {
		t.Helper()
		resp, err := http.Post(nearSide.URL+"/remotes/open", "application/json",
			strings.NewReader(`{"host":"gpu-box","workspace":"`+shared+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("open remote = %d: %s", resp.StatusCode, body)
		}
		var view RuntimeView
		if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
		return view
	}

	first, second := open(), open()
	if n := len(far.List()); n != 2 {
		t.Fatalf("the far hub holds %d runtimes for two panes, want 2", n)
	}
	// Bound to its own runtime over there, not to whichever answered first.
	a, aok := near.Get(first.ID).Remote()
	b, bok := near.Get(second.ID).Remote()
	if !aok || !bok || a.RemoteID == "" || a.RemoteID == b.RemoteID {
		t.Fatalf("panes bound to remote runtimes %q and %q", a.RemoteID, b.RemoteID)
	}
	// And the proxy actually reaches them: a path that missed would 404 rather
	// than answer, which is what a prefix built wrong looks like.
	if root := remotePaneRoot(t, nearSide, first); root != shared {
		t.Fatalf("pane answered for %q, want the workspace it opened (%q)", root, shared)
	}
}

// remotePaneRoot asks a pane which workspace its kernel is driving.
func remotePaneRoot(t *testing.T, front *httptest.Server, view RuntimeView) string {
	t.Helper()
	resp, err := http.Get(front.URL + view.Base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status via pane = %d: %s", resp.StatusCode, body)
	}
	var status struct {
		WorkspaceRoot string `json:"workspaceRoot"`
		Cwd           string `json:"cwd"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.WorkspaceRoot != "" {
		return status.WorkspaceRoot
	}
	return status.Cwd
}

// writeOpenableConfig gives a hub the one thing Open needs: a provider to
// assemble against. Nothing dials it — the address is deliberately dead.
func writeOpenableConfig(t *testing.T) {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	closeSharedCatalogsOnCleanup(t)
	path := config.UserConfigPath()
	if path == "" {
		t.Fatal("user config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `default_model = "default/shared-chat"

[[providers]]
name = "default"
kind = "openai"
base_url = "http://127.0.0.1:1/v1"
models = ["shared-chat"]
default = "shared-chat"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A pane closed here has to close the one it was driving over there, or a
// machine collects runtimes until its own ceiling refuses the next connect.
func TestClosingARemotePaneRetiresTheFarRuntime(t *testing.T) {
	writeOpenableConfig(t)
	far := NewHub(HubOptions{})
	defer far.Shutdown()
	farSide := httptest.NewServer(far.Handler())
	defer farSide.Close()

	near := NewHub(HubOptions{Remote: &stubAttacher{
		attach: func(host, workspace string) (RemoteEndpoint, func(), error) {
			return RemoteEndpoint{
				Host: host, Workspace: workspace,
				Addr: farSide.Listener.Addr().String(), Token: "t",
			}, func() {}, nil
		},
	}})
	nearSide := httptest.NewServer(near.Handler())
	defer nearSide.Close()

	resp, err := http.Post(nearSide.URL+"/remotes/open", "application/json",
		strings.NewReader(`{"host":"gpu-box","workspace":"`+t.TempDir()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var view RuntimeView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if len(far.List()) != 1 {
		t.Fatalf("the far hub holds %d runtimes after one open, want 1", len(far.List()))
	}

	if err := near.Close(view.ID); err != nil {
		t.Fatalf("close pane: %v", err)
	}
	if n := len(far.List()); n != 0 {
		t.Fatalf("the far hub still holds %d runtimes after the pane closed", n)
	}
}

// A Windows workspace reaches here spelled the way that machine spells it.
// Cutting it with this machine's rules — or with one separator — returns the
// whole string as the pane's name, which is what the tab would then show.
func TestRemotePaneNameIsCutTheRemoteWay(t *testing.T) {
	for spelled, want := range map[string]string{
		`C:\Users\ada\training`: "training",
		"/srv/data/training":    "training",
		`C:\Users\ada\`:         "ada",
		`C:\`:                   "C:",
	} {
		if got := remoteBaseName(spelled); got != want {
			t.Fatalf("remoteBaseName(%q) = %q, want %q", spelled, got, want)
		}
	}
}
