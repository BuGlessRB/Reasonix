package boot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/platform/feedback"
)

// Through the real assembly: sending feedback is a host action. What the
// person typed goes to the feedback service redacted, and nothing about it —
// not the text, not a notice, not a tool — ever reaches a model request, so
// the cache-stable prefix is the one a session without feedback would send.
func TestEffectFeedbackNeverReachesTheModelOrMovesThePrefix(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	rec := &browserScriptProvider{}
	kind := "boot-feedback-probe"
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"
tool_approval = "yolo"

[agent]
system_prompt = "BASE"

[codegraph]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	approveWorkspace(t, dir)

	var mu sync.Mutex
	var posts []map[string]any
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		mu.Lock()
		posts = append(posts, got)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"receipt":"FB-7K3M-9QX2","status":"received","installToken":"t","createdAt":"2026-09-30T08:00:00Z"}`)
	}))
	defer svc.Close()
	t.Setenv("REASONIX_FEEDBACK_URL", svc.URL)

	var notices []string
	var nmu sync.Mutex
	ctrl, err := Build(context.Background(), Options{
		FeedbackSurface: feedback.SurfaceTUI,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice {
				nmu.Lock()
				notices = append(notices, e.Text)
				nmu.Unlock()
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()

	if err := ctrl.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.SetFeedbackDisplayName("kim"); err != nil {
		t.Fatal(err)
	}
	const marker = "MARKER-the-sidebar-forgets-me"
	ctrl.Submit("/feedback bug --yes " + marker + " api_key=sk-abcdefghijklmnopqrstuvwxyz")
	deadline := time.Now().Add(10 * time.Second)
	for {
		nmu.Lock()
		done := strings.Contains(strings.Join(notices, "\n"), "Receipt FB-7K3M-9QX2")
		nmu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no receipt notice; notices = %q", notices)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := ctrl.Run(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(posts) != 1 {
		t.Fatalf("the service saw %d posts", len(posts))
	}
	sent := posts[0]
	if body := sent["body"].(string); !strings.Contains(body, marker) || strings.Contains(body, "sk-abcdef") {
		t.Fatalf("body on the wire = %q", body)
	}
	if env := sent["env"].(map[string]any); env["surface"] != "tui" || env["providerKind"] != "other" {
		t.Fatalf("env on the wire = %v", env)
	}

	reqs := rec.requests()
	if len(reqs) != 2 {
		t.Fatalf("the model saw %d requests: /feedback must not start a turn", len(reqs))
	}
	for i, r := range reqs {
		raw, _ := json.Marshal(r)
		for _, leak := range []string{marker, "FB-7K3M", "sk-abcdef"} {
			if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(leak)) {
				t.Errorf("request %d carries %q", i, leak)
			}
		}
	}
	if !reflect.DeepEqual(reqs[0].Tools, reqs[1].Tools) {
		t.Error("the tool schema moved between the turns around a /feedback")
	}
	if n := len(reqs[0].Messages); len(reqs[1].Messages) < n || !reflect.DeepEqual(reqs[1].Messages[:n], reqs[0].Messages) {
		t.Error("the message prefix moved between the turns around a /feedback")
	}
}
