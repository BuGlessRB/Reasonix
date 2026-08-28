package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/testenv"
)

// The panel behind this endpoint used to infer pending changes from tool events,
// which cannot see a file a shell command removed. /changes answers from the
// tree, and says repo=false rather than "nothing changed" when there is no git.
func TestChangesReportsTheTreeNotTheTranscript(t *testing.T) {
	dir := testenv.TempDir(t)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: dir})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	var body struct {
		Repo    bool `json:"repo"`
		Changes []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"changes"`
	}
	read := func() {
		t.Helper()
		resp, err := http.Get(srv.URL + "/changes")
		if err != nil {
			t.Fatalf("GET /changes: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /changes = %d", resp.StatusCode)
		}
		body.Changes = nil
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}

	read()
	if body.Repo {
		t.Fatalf("a workspace with no git should report repo=false, got %+v", body)
	}

	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "base", "--allow-empty"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v\n%s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "scratch.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read()
	if !body.Repo || len(body.Changes) != 1 || body.Changes[0].Path != "scratch.go" {
		t.Fatalf("after create: %+v, want one untracked scratch.go", body)
	}
	if err := os.Remove(filepath.Join(dir, "scratch.go")); err != nil {
		t.Fatal(err)
	}
	read()
	if len(body.Changes) != 0 {
		t.Fatalf("after delete: %+v, want no pending change", body)
	}
}
