package browser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// hostedBrowser is the smallest host a Session can drive: the commands Open
// sends, answered the way a browser would, with the load event it waits for.
type hostedBrowser struct {
	in      chan []byte
	mu      sync.Mutex
	methods []string
	closed  chan struct{}
	once    sync.Once
}

func newHostedBrowser() *hostedBrowser {
	return &hostedBrowser{in: make(chan []byte, 64), closed: make(chan struct{})}
}

func (h *hostedBrowser) ReadMessage() ([]byte, error) {
	select {
	case m := <-h.in:
		return m, nil
	case <-h.closed:
		return nil, io.EOF
	}
}

func (h *hostedBrowser) Close() error {
	h.once.Do(func() { close(h.closed) })
	return nil
}

func (h *hostedBrowser) send(v any) {
	raw, _ := json.Marshal(v)
	h.in <- raw
}

func (h *hostedBrowser) WriteMessage(raw []byte) error {
	var msg wireMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	h.mu.Lock()
	h.methods = append(h.methods, msg.Method)
	h.mu.Unlock()
	result := map[string]any{}
	switch msg.Method {
	case "Target.createTarget":
		result["targetId"] = "view-1"
	case "Target.attachToTarget":
		result["sessionId"] = "S-view-1"
	case "Page.getFrameTree":
		result["frameTree"] = map[string]any{"frame": map[string]any{"id": "F1", "url": "about:blank"}}
	case "Page.navigate":
		result["loaderId"] = "L1"
		defer h.send(map[string]any{"method": "Page.lifecycleEvent", "sessionId": msg.SessionID,
			"params": map[string]any{"frameId": "F1", "loaderId": "L1", "name": "load"}})
	case "Runtime.evaluate":
		result["result"] = map[string]any{"value": "Hosted"}
	}
	h.send(map[string]any{"id": msg.ID, "result": result})
	return nil
}

func TestSessionDrivesAHostedBrowserWithoutLaunchingOne(t *testing.T) {
	host := newHostedBrowser()
	pool := &Pool{}
	var dialed string
	pool.SetEndpoint(func(_ context.Context, profile string) (Endpoint, error) {
		dialed = profile
		return host, nil
	})
	s := NewSession(Config{Launch: LaunchSpec{Executable: "/nonexistent/chrome", ProfileDir: "/profiles/w1"}, Pool: pool})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := s.Open(ctx, "https://example.com/", "", false)
	if err != nil {
		t.Fatalf("Open through the host: %v", err)
	}
	if info.Title != "Hosted" || dialed != "/profiles/w1" {
		t.Fatalf("info = %+v, dialed %q", info, dialed)
	}
	s.Close()
	host.mu.Lock()
	defer host.mu.Unlock()
	for _, want := range []string{"Browser.setDownloadBehavior", "Target.closeTarget", "Browser.close"} {
		found := false
		for _, m := range host.methods {
			found = found || m == want
		}
		if !found {
			t.Errorf("the host never received %s: %v", want, host.methods)
		}
	}
}

func TestNoHostFallsBackToLaunching(t *testing.T) {
	pool := &Pool{}
	pool.SetEndpoint(func(context.Context, string) (Endpoint, error) { return nil, ErrNoHost })
	s := NewSession(Config{Launch: LaunchSpec{Executable: "/nonexistent/chrome", ProfileDir: t.TempDir()}, Pool: pool})
	if _, err := s.Open(context.Background(), "https://example.com/", "", false); CodeOf(err) != CodeEngineMissing {
		t.Fatalf("with no host attached = %v, want the launch path's %s", err, CodeEngineMissing)
	}
	pool.SetEndpoint(func(context.Context, string) (Endpoint, error) { return nil, errors.New("refused") })
	s = NewSession(Config{Launch: LaunchSpec{Executable: "/nonexistent/chrome", ProfileDir: t.TempDir()}, Pool: pool})
	if _, err := s.Open(context.Background(), "https://example.com/", "", false); CodeOf(err) != CodeEngineFailed {
		t.Fatalf("a host that refuses = %v, want %s rather than a silent launch", err, CodeEngineFailed)
	}
}
