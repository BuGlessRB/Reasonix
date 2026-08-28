package serve

import (
	"context"
	"encoding/json"
	"fmt"
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
	"reasonix/internal/surface"
	"reasonix/internal/testenv"
)

func hubRuntime(t *testing.T, h *Hub, root string) *Runtime {
	t.Helper()
	ctrl := control.New(control.Options{WorkspaceRoot: root, SessionDir: SessionDirFor(root)})
	t.Cleanup(ctrl.Close)
	bc := NewBroadcaster()
	rt, err := h.Adopt(New(ctrl, bc, config.ServeConfig{}), bc)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	return rt
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	first, second := testenv.TempDir(t), testenv.TempDir(t)
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, testenv.TempDir(t))
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
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
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	root := testenv.TempDir(t)
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
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
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
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

// A save that keeps conflicting leaves one copy per turn, every one of them
// carrying the first message as its title. They fold into the conversation they
// came from, or the sidebar fills with rows the user never made.
func TestHubTreeFoldsRecoveryCopiesIntoTheirConversation(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	dir := SessionDirFor(root)

	origin := filepath.Join(dir, "20260817-090000-deepseek-v4-flash.jsonl")
	live := agent.NewSession("sys")
	live.Add(provider.Message{Role: provider.RoleUser, Content: `"C:\Users\FuChen\report.txt"`})
	live.Add(provider.Message{Role: provider.RoleAssistant, Content: "好的"})
	if err := live.Save(origin); err != nil {
		t.Fatal(err)
	}
	// The origin moves on to content no runtime had, so no copy is covered by
	// its parent and none of them is hidden.
	outside, err := agent.LoadSession(origin)
	if err != nil {
		t.Fatal(err)
	}
	outside.Add(provider.Message{Role: provider.RoleUser, Content: "外部写入"})
	if err := outside.Save(origin); err != nil {
		t.Fatal(err)
	}

	// Three panes that resumed the same conversation and each took it somewhere
	// else. A branch is taken over only by a save that keeps every turn already
	// in it, so transcripts that diverge cannot share one — the pile to fold.
	base := live.Snapshot()
	parent := origin
	var copies []string
	for turn := range 3 {
		conflicted := agent.NewSession("sys")
		for _, m := range base {
			if m.Role != provider.RoleSystem {
				conflicted.Add(m)
			}
		}
		conflicted.Add(provider.Message{Role: provider.RoleUser, Content: fmt.Sprintf("第 %d 轮", turn)})
		conflicted.Add(provider.Message{Role: provider.RoleAssistant, Content: "好"})
		info, err := conflicted.SaveRecoveryBranch(agent.RecoveryBranchOptions{OriginalPath: parent})
		if err != nil {
			t.Fatal(err)
		}
		copies = append(copies, info.Path)
		parent = info.Path
	}

	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	tree := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(tree) != 1 {
		t.Fatalf("/tree = %+v, want the one workspace", tree)
	}
	rows := tree[0].Sessions
	if len(rows) != 2 {
		t.Fatalf("sidebar shows %d rows (%+v), want the conversation and one folded copy row", len(rows), rows)
	}
	var folded treeSession
	for _, row := range rows {
		if len(row.Copies) > 0 {
			folded = row
		}
	}
	if folded.Path != copies[len(copies)-1] {
		t.Fatalf("folded row is %q, want the newest copy %q", folded.Path, copies[len(copies)-1])
	}
	if len(folded.Copies) != len(copies)-1 {
		t.Fatalf("row carries %d copies, want %d", len(folded.Copies), len(copies)-1)
	}
	for _, copy := range folded.Copies {
		if copy.Turns == 0 || copy.Path == "" {
			t.Fatalf("folded copy %+v needs a path and turn count to be openable", copy)
		}
	}
}

// A window's turns must not be filed under the frontend it happens to be built
// on. The hub stamps its surface where a server enters it, so neither of the
// two doors can be the one that forgets.
func TestHubStampsItsSurfaceOnEveryServerItAdopts(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts HubOptions
		want surface.Surface
	}{
		{"a hub nobody claimed is the bare server", HubOptions{}, surface.Serve},
		{"a window says which frontend it is", HubOptions{Surface: surface.Desktop}, surface.Desktop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := testenv.TempDir(t)
			h := NewHub(tc.opts)
			t.Cleanup(h.Shutdown)
			ctrl := control.New(control.Options{WorkspaceRoot: root, SessionDir: SessionDirFor(root)})
			t.Cleanup(ctrl.Close)
			bc := NewBroadcaster()
			srv := New(ctrl, bc, config.ServeConfig{})
			if _, err := h.Adopt(srv, bc); err != nil {
				t.Fatalf("adopt: %v", err)
			}
			if got := srv.statsSurface(); got != tc.want {
				t.Fatalf("adopted server records as %q, want %q", got, tc.want)
			}
		})
	}
}

// A server no hub ever claimed still has to label the records its own rebuilds
// write, and the bare frontend is the only thing it can be.
func TestUnclaimedServerRecordsAsServe(t *testing.T) {
	root := testenv.TempDir(t)
	ctrl := control.New(control.Options{WorkspaceRoot: root, SessionDir: SessionDirFor(root)})
	t.Cleanup(ctrl.Close)
	if got := New(ctrl, NewBroadcaster(), config.ServeConfig{}).statsSurface(); got != surface.Serve {
		t.Fatalf("unclaimed server records as %q, want %q", got, surface.Serve)
	}
}
