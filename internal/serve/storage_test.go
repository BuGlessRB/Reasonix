package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/testenv"
)

func readStorage(t *testing.T, base string) map[string]any {
	t.Helper()
	resp, err := http.Get(base + "/storage")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /storage = %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func storageRootsOf(t *testing.T, body map[string]any) map[string]map[string]any {
	t.Helper()
	rows, ok := body["roots"].([]any)
	if !ok {
		t.Fatalf("no roots in %v", body)
	}
	out := map[string]map[string]any{}
	for _, row := range rows {
		entry, ok := row.(map[string]any)
		if !ok {
			t.Fatalf("root row = %T", row)
		}
		out[entry["id"].(string)] = entry
	}
	return out
}

// The panel enumerates whatever the kernel declares, so every root has to
// arrive — including the ones it may only show and never offer to move.
func TestStorageListsEveryRootWithItsRelocationFacts(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	roots := storageRootsOf(t, readStorage(t, srv.URL))
	for _, id := range config.RootIDs() {
		row, ok := roots[string(id)]
		if !ok {
			t.Fatalf("root %q missing from /storage", id)
		}
		if row["relocatable"] != config.RootRelocatable(id) {
			t.Fatalf("root %q reported relocatable=%v", id, row["relocatable"])
		}
	}
	if roots["locks"]["relocatable"] != false {
		t.Fatal("the locks root was offered as movable over the wire")
	}
}

// A plan is the answer to "what would this cost", and it must be reachable
// without doing anything: the panel shows the cost before a person commits.
func TestStoragePlanReportsCostAndRefusalsWithoutMoving(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	before := config.RootDir(config.RootState)
	resp := postProvider(t, srv.URL, "/storage/plan", `{"root":"state","dir":""}`)
	defer resp.Body.Close()
	var plan map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan["ok"] != false {
		t.Fatalf("an empty target planned as ok: %v", plan)
	}
	refusals, _ := plan["refusals"].([]any)
	if len(refusals) == 0 {
		t.Fatal("a refused plan carried no reason")
	}
	first := refusals[0].(map[string]any)
	if first["code"] == "" || first["detail"] == "" {
		t.Fatalf("refusal missing a code or a sentence: %v", first)
	}
	if got := config.RootDir(config.RootState); got != before {
		t.Fatalf("planning moved the root to %q", got)
	}
}

// Relocation writes the configuration of the machine running the kernel, which
// is the one thing a server reachable over a network must not let a client do.
func TestStorageMoveIsRefusedUntilTheHostGrantsIt(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/storage/move", `{"root":"state","dir":"`+storagePathLiteral(testenv.TempDir(t))+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /storage/move without the grant = %d, want 403", resp.StatusCode)
	}
}

// The move runs past the request that started it and reports through the same
// listing the panel already reads.
func TestStorageMoveRunsAndReportsThroughTheListing(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	home := config.RootDir(config.RootState)
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "sessions", "a.jsonl"), make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(testenv.TempDir(t), "moved")
	t.Cleanup(config.InvalidateStorageDirs)

	resp := postProvider(t, srv.URL, "/storage/move", `{"root":"state","dir":"`+storagePathLiteral(target)+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readAllString(resp)
		t.Fatalf("POST /storage/move = %d: %s", resp.StatusCode, body)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		move, _ := readStorage(t, srv.URL)["move"].(map[string]any)
		if move != nil && move["done"] == true {
			if errText, _ := move["err"].(string); errText != "" {
				t.Fatalf("move failed: %s", errText)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("move never finished: %v", move)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(target, "sessions", "a.jsonl")); err != nil {
		t.Fatalf("the transcript did not arrive: %v", err)
	}
}

// storagePathLiteral escapes a Windows path for embedding in a JSON string.
func storagePathLiteral(p string) string {
	b, err := json.Marshal(p)
	if err != nil {
		return p
	}
	return string(b[1 : len(b)-1])
}
