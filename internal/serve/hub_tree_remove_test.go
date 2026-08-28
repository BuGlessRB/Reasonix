package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
	"reasonix/internal/store"
	"reasonix/internal/testenv"
)

func writeSessionAt(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	s := agent.NewSession("sys")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
}

func postRemoveSession(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/tree/sessions/remove", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// A conversation something is still writing to must not be deleted, even when
// it is not the transcript any pane is currently pointed at — a recovery branch
// and a session mid-rotation are both held without being anyone's current path.
// The pane map cannot see either, so the lease is what has to answer.
func TestRemoveSessionRefusesALeasedTranscript(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	path := filepath.Join(SessionDirFor(root), "held.jsonl")
	writeSessionAt(t, path)

	lease, err := agent.TryAcquireSessionLease(path)
	if err != nil {
		t.Fatalf("TryAcquireSessionLease: %v", err)
	}
	defer lease.Release()

	resp := postRemoveSession(t, srv, path)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("POST /tree/sessions/remove = %d, want 409 while the lease is held", resp.StatusCode)
	}
	// The code is the only part a reader ever sees: the frontend looks up its
	// own wording by it, and falls back to printing this response otherwise.
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if body.Code != "session.in_use" {
		t.Errorf("refusal code = %q, want session.in_use", body.Code)
	}
	// Refused means refused: not one byte may be gone.
	for _, p := range []string{path, store.SessionEventLog(path)} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("artifact erased despite the refusal: %s (%v)", p, err)
		}
	}
	if agent.IsCleanupPending(path) {
		t.Error("a refused delete must not mark the session for cleanup")
	}
}

// The same session deletes normally once nothing holds it, so the guard cannot
// be a session that can never be removed.
func TestRemoveSessionSucceedsOnceTheLeaseIsReleased(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	path := filepath.Join(SessionDirFor(root), "free.jsonl")
	writeSessionAt(t, path)

	lease, err := agent.TryAcquireSessionLease(path)
	if err != nil {
		t.Fatalf("TryAcquireSessionLease: %v", err)
	}
	lease.Release()

	resp := postRemoveSession(t, srv, path)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /tree/sessions/remove = %d, want 204 after release", resp.StatusCode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("transcript survived the delete: %v", err)
	}
	if _, err := os.Stat(store.SessionEventLog(path)); !os.IsNotExist(err) {
		t.Error("event log survived the delete, so the conversation could be resurrected")
	}
}
