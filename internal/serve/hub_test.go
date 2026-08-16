package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/provider"
)

func hubRuntime(t *testing.T, h *Hub, root string) *Runtime {
	t.Helper()
	ctrl := control.New(control.Options{WorkspaceRoot: root, SessionDir: SessionDirFor(root)})
	t.Cleanup(ctrl.Close)
	bc := NewBroadcaster()
	return h.Adopt(New(ctrl, bc, config.ServeConfig{}), bc)
}

func hubGet[T any](t *testing.T, srv *httptest.Server, path string) T {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Two panes must answer for their own session. Sharing one server is what made
// a second conversation rebuild the first instead of running beside it.
func TestHubRoutesEachRuntimeToItsOwnWorkspace(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	first, second := t.TempDir(), t.TempDir()
	h := NewHub(HubOptions{})
	a := hubRuntime(t, h, first)
	b := hubRuntime(t, h, second)
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	for _, want := range []struct {
		rt   *Runtime
		root string
	}{{a, first}, {b, second}} {
		got := hubGet[struct {
			WorkspaceRoot string `json:"workspaceRoot"`
		}](t, srv, "/rt/"+want.rt.ID+"/status")
		if got.WorkspaceRoot != want.root {
			t.Fatalf("/rt/%s/status workspaceRoot = %q, want %q", want.rt.ID, got.WorkspaceRoot, want.root)
		}
	}
	if views := h.List(); len(views) != 2 || views[0].Base != "/rt/"+a.ID {
		t.Fatalf("List() = %+v, want both runtimes in open order", views)
	}
}

// One transcript, one writer. Opening a session a pane already drives has to
// focus that pane — two runtimes on one file fork a recovery branch per save.
func TestHubOpenFocusesTheRuntimeAlreadyDrivingTheSession(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	root := t.TempDir()
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, root)
	path := filepath.Join(SessionDirFor(root), "20260815-161507-deepseek-v4-flash.jsonl")
	rt.Server.Controller().SetSessionPath(path)

	got, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if got != rt {
		t.Fatalf("Open returned runtime %q, want the one already driving the session (%q)", got.ID, rt.ID)
	}
	if len(h.List()) != 1 {
		t.Fatalf("hub holds %d runtimes, want the one", len(h.List()))
	}
}

// The sidebar is one request: every workspace, its saved conversations, and
// which of them a pane already has open.
func TestHubTreeListsWorkspaceSessionsAndMarksOpenOnes(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	root := t.TempDir()
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, root)

	dir := SessionDirFor(root)
	open := filepath.Join(dir, "20260815-161507-deepseek-v4-flash.jsonl")
	idle := filepath.Join(dir, "20260815-090000-deepseek-v4-flash.jsonl")
	for _, path := range []string{open, idle} {
		s := agent.NewSession("sys")
		s.Add(provider.Message{Role: provider.RoleUser, Content: "今日热点"})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: "好的"})
		if err := s.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	rt.Server.Controller().SetSessionPath(open)

	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	tree := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(tree) != 1 || tree[0].Root != root {
		t.Fatalf("/tree = %+v, want the one workspace", tree)
	}
	if !tree[0].Open {
		t.Fatalf("workspace %q must report a pane driving it", tree[0].Root)
	}
	byPath := map[string]treeSession{}
	for _, s := range tree[0].Sessions {
		byPath[s.Path] = s
	}
	if got := byPath[open]; got.RuntimeID != rt.ID {
		t.Fatalf("open session runtimeId = %q, want %q", got.RuntimeID, rt.ID)
	}
	if got, ok := byPath[idle]; !ok || got.RuntimeID != "" {
		t.Fatalf("idle session = %+v, want it listed with no runtime", got)
	}
}

// Closing a pane retires its runtime: the address stops answering and the
// session is free for another window to open.
func TestHubCloseRetiresTheRuntime(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, t.TempDir())
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	if err := h.Close(rt.ID); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/rt/" + rt.ID + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET a closed runtime = %d, want 404", resp.StatusCode)
	}
	if len(h.List()) != 0 {
		t.Fatalf("hub still lists %d runtimes", len(h.List()))
	}
}

// A client that knows nothing about runtimes — a browser opened straight at the
// port — still reaches the first one.
func TestHubServesTheFirstRuntimeUnprefixed(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	root := t.TempDir()
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	got := hubGet[struct {
		WorkspaceRoot string `json:"workspaceRoot"`
	}](t, srv, "/status")
	if got.WorkspaceRoot != root {
		t.Fatalf("unprefixed /status workspaceRoot = %q, want %q", got.WorkspaceRoot, root)
	}
}

// Closing the last pane and then asking for a new session is an ordinary
// sequence — deleting the only conversation does exactly that. With nothing
// open there is no runtime to infer the folder from, and the request used to
// come back "missing root".
func TestHubOpensInARememberedWorkspaceWithNoPanesLeft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := t.TempDir()
	rememberWorkspace(root)

	h := NewHub(HubOptions{})
	got, err := h.resolveRoot(OpenRequest{})
	if err != nil {
		t.Fatalf("resolveRoot with no panes: %v", err)
	}
	if got != root {
		t.Fatalf("resolveRoot = %q, want the remembered folder %q", got, root)
	}
}

// With nothing remembered either, the refusal has to say what is missing.
func TestHubRefusalNamesTheMissingFolder(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	h := NewHub(HubOptions{})
	_, err := h.resolveRoot(OpenRequest{})
	if err == nil || !strings.Contains(err.Error(), "add a folder") {
		t.Fatalf("err = %v, want it to point at adding a folder", err)
	}
}

// The ceiling is a machine's own call: a laptop with no MCP servers holds far
// more panes than one with five, so the number is configurable and the list
// carries it. A client that hardcoded 8 would grey out its control at the wrong
// count the moment the config differs.
func TestPaneCeilingComesFromConfig(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	path := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[desktop]\nmax_panes = 12\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := maxRuntimes(); got != 12 {
		t.Fatalf("maxRuntimes = %d, want 12", got)
	}
	// Clamped rather than trusted: an unbounded pane count is an unbounded
	// process however the number got into the file.
	if err := os.WriteFile(path, []byte("[desktop]\nmax_panes = 900\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := maxRuntimes(); got != 32 {
		t.Fatalf("maxRuntimes with an absurd value = %d, want it clamped to 32", got)
	}
	if err := os.WriteFile(path, []byte("[desktop]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := maxRuntimes(); got != maxRuntimesDefault {
		t.Fatalf("maxRuntimes unset = %d, want the default %d", got, maxRuntimesDefault)
	}
}
