package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

func readJSON[T any](t *testing.T, base, path string) T {
	t.Helper()
	resp, err := http.Get(base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", path, resp.StatusCode)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Widening what the agent may do to this machine is a write to this machine, so
// both routes ride the provider-edit grant. Without it a networked client could
// hand itself the boundary it is supposed to be held by.
func TestBoundaryWritesRefusedWithoutGrant(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{"/permissions", "/sandbox"} {
		resp := postProvider(t, srv.URL, path, `{}`)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("POST %s without the grant = %d, want 403", path, resp.StatusCode)
		}
	}
}

func TestSavePermissionsPersistsAllThreeLists(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := `{"mode":"deny","deny":["bash(git push:*)"],"ask":["bash(rm:*)"],"allow":["bash(go test:*)"]}`
	resp := postProvider(t, srv.URL, "/permissions", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		got, _ := readAllString(resp)
		t.Fatalf("POST /permissions = %d: %s", resp.StatusCode, got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Permissions.Mode != "deny" {
		t.Fatalf("persisted mode = %q, want deny", cfg.Permissions.Mode)
	}
	if len(cfg.Permissions.Deny) != 1 || cfg.Permissions.Deny[0] != "bash(git push:*)" {
		t.Fatalf("persisted deny = %v", cfg.Permissions.Deny)
	}
	if len(cfg.Permissions.Ask) != 1 || len(cfg.Permissions.Allow) != 1 {
		t.Fatalf("ask = %v, allow = %v", cfg.Permissions.Ask, cfg.Permissions.Allow)
	}

	got := readJSON[control.PermissionRules](t, srv.URL, "/permissions")
	if got.Mode != "deny" || len(got.Deny) != 1 {
		t.Fatalf("read back = %+v", got)
	}
	if got.Path == "" {
		t.Fatal("read back names no config file, so the pane cannot say where an edit lands")
	}
}

// A rule the gate's own parser rejects never reaches the config: it would sit in
// the list looking enforced and match nothing at all.
func TestSavePermissionsRejectsUnparsableRule(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/permissions", `{"mode":"ask","deny":["(no tool name)"]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /permissions with a bad rule = %d, want 400", resp.StatusCode)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Permissions.Deny) != 0 {
		t.Fatalf("rejected rule was persisted: %v", cfg.Permissions.Deny)
	}
}

// Replacing the lists must not leave yesterday's rules behind: the editor sends
// what the screen shows, and anything kept silently is a boundary nobody chose.
func TestSavePermissionsReplacesRatherThanAppends(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	first := postProvider(t, srv.URL, "/permissions", `{"mode":"ask","deny":["bash(rm:*)","bash(git push:*)"]}`)
	first.Body.Close()
	second := postProvider(t, srv.URL, "/permissions", `{"mode":"ask","deny":["bash(rm:*)"]}`)
	second.Body.Close()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Permissions.Deny) != 1 || cfg.Permissions.Deny[0] != "bash(rm:*)" {
		t.Fatalf("deny after removal = %v, want just bash(rm:*)", cfg.Permissions.Deny)
	}
}

func TestSandboxSettingsReportWhatTheConfinerWillUse(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/sandbox", `{"bash":"off","network":false,"allowWrite":["/tmp/scratch"," "]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		got, _ := readAllString(resp)
		t.Fatalf("POST /sandbox = %d: %s", resp.StatusCode, got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Bash != "off" || cfg.Sandbox.Network {
		t.Fatalf("persisted jail = %+v", cfg.Sandbox)
	}
	if len(cfg.Sandbox.AllowWrite) != 1 || cfg.Sandbox.AllowWrite[0] != "/tmp/scratch" {
		t.Fatalf("allowWrite = %v, want the blank entry dropped", cfg.Sandbox.AllowWrite)
	}

	// An empty workspace root is not "anywhere": the expansion still names a
	// directory, and the pane shows that rather than a blank.
	got := readJSON[control.SandboxSettings](t, srv.URL, "/sandbox")
	if len(got.EffectiveWriteRoots) == 0 {
		t.Fatal("no effective write roots reported")
	}
	for _, root := range got.EffectiveWriteRoots {
		if root == "" {
			t.Fatalf("effective roots contain a blank: %v", got.EffectiveWriteRoots)
		}
	}
}

func TestSaveSandboxRejectsUnknownBashMode(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/sandbox", `{"bash":"maybe"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /sandbox with an unknown mode = %d, want 400", resp.StatusCode)
	}
}
