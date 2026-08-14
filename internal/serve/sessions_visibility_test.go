package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

// A subagent transcript is a task's internal trace, not one of the user's
// conversations. Listing them buried the real sessions: 154 of 189 entries in a
// real profile were subagents.
func TestSessionsExcludesSubagentTranscripts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"20260813-235122-deepseek-v4-flash.jsonl",
		"subagent-sub-k-202605171512.jsonl",
		"subagent-sub-1-202605171456.jsonl",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctrl := control.New(control.Options{SessionDir: dir})
	defer ctrl.Close()
	ctrl.SetSessionPath(filepath.Join(dir, "20260813-235122-deepseek-v4-flash.jsonl"))

	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("GET /sessions returned %d entries, want only the real session: %+v", len(got), got)
	}
	if got[0].Name != "20260813-235122-deepseek-v4-flash" {
		t.Errorf("listed %q, want the real session", got[0].Name)
	}
}
