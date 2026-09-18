package workspacestate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureWorkspaceResolvedReturnsExistingPhysicalOwner(t *testing.T) {
	realRoot := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "workspace-state-v1.json"))
	if err := store.EnsureWorkspace(t.Context(), Workspace{ID: "legacy-project", Root: realRoot, Title: "kept"}); err != nil {
		t.Fatal(err)
	}
	id, err := store.EnsureWorkspaceResolved(t.Context(), Workspace{ID: "new-candidate", Root: alias, Title: "ignored"})
	if err != nil || id != "legacy-project" {
		t.Fatalf("resolved id = %q, err = %v", id, err)
	}
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Workspaces) != 1 || state.Workspaces[id].Root != realRoot || state.Workspaces[id].Title != "kept" {
		t.Fatalf("state was rewritten: %+v", state.Workspaces)
	}
}

func TestEnsureWorkspaceResolvedRejectsAmbiguousLegacyOwners(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	path := filepath.Join(t.TempDir(), "workspace-state-v1.json")
	state := newState()
	state.Initialized = true
	state.WorkspaceIDs = []string{"old-a", "old-b"}
	state.Workspaces["old-a"] = Workspace{ID: "old-a", Root: root, SessionIDs: []string{}}
	state.Workspaces["old-b"] = Workspace{ID: "old-b", Root: alias, SessionIDs: []string{}}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), body...)
	_, err = NewStore(path).EnsureWorkspaceResolved(t.Context(), Workspace{ID: "candidate", Root: root})
	if !errors.Is(err, ErrAmbiguousIdentity) {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || strings.TrimSpace(string(after)) != string(before) {
		t.Fatalf("ambiguous state changed: %v", readErr)
	}
}

func TestEnsureWorkspaceResolvedUsesVersionedIDForRealCollision(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "workspace-state-v1.json"))
	if err := store.EnsureWorkspace(t.Context(), Workspace{ID: "project-collision", Root: filepath.Join(t.TempDir(), "one")}); err != nil {
		t.Fatal(err)
	}
	id, err := store.EnsureWorkspaceResolved(t.Context(), Workspace{ID: "project-collision", Root: filepath.Join(t.TempDir(), "two")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "project-v2-") {
		t.Fatalf("fallback id = %q", id)
	}
}

func TestEnsureWorkspaceAcceptsEquivalentRootWithoutRewritingState(t *testing.T) {
	for _, id := range []string{GlobalWorkspaceID, "project-a"} {
		t.Run(id, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Workspace")
			path := filepath.Join(t.TempDir(), "workspace-state-v1.json")
			store := NewStore(path)
			if err := store.EnsureWorkspace(t.Context(), Workspace{ID: id, Root: root, Title: "My workspace", Visible: false}); err != nil {
				t.Fatal(err)
			}
			if err := store.AttachSession(t.Context(), "", id, "saved-session", ""); err != nil {
				t.Fatal(err)
			}
			if err := store.ArchiveSession(t.Context(), "saved-session"); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			aliases := []string{root + string(os.PathSeparator) + ".", root + string(os.PathSeparator)}
			if runtime.GOOS == "windows" {
				aliases = append(aliases, strings.ToUpper(root), strings.ReplaceAll(root, `\`, "/"))
			}
			for _, alias := range aliases {
				// Reopen the registry just as a later process would. An equivalent
				// spelling must preserve the old root, presentation and lifecycle.
				if err := NewStore(path).EnsureWorkspace(t.Context(), Workspace{ID: id, Root: alias, Title: "Default", Visible: true}); err != nil {
					t.Fatalf("equivalent root %q: %v", alias, err)
				}
				after, err := os.ReadFile(path)
				if err != nil || string(after) != string(before) {
					t.Fatalf("equivalent root rewrote persisted state: %v", err)
				}
			}
		})
	}
}

func TestEnsureWorkspaceRejectsDifferentRootWithoutRewritingState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Workspace")
	store := NewStore(filepath.Join(t.TempDir(), "workspace-state-v1.json"))
	if err := store.EnsureWorkspace(t.Context(), Workspace{ID: GlobalWorkspaceID, Root: root}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	roots := []string{"", filepath.Join(root, "different")}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		roots = append(roots, strings.ToUpper(root))
	}
	for _, other := range roots {
		if err := store.EnsureWorkspace(t.Context(), Workspace{ID: GlobalWorkspaceID, Root: other}); !errors.Is(err, ErrMutationConflict) {
			t.Fatalf("different root %q: %v", other, err)
		}
	}
	after, err := os.ReadFile(store.Path())
	if err != nil || string(after) != string(before) {
		t.Fatalf("conflicting root rewrote persisted state: %v", err)
	}
}
