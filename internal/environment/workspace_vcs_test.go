package environment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/fileutil"
)

func TestWorkspaceVCSFindsRepositoryFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	nested := filepath.Join(root, "internal", "agent")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := WorkspaceVCS(nested); got != "git" {
		t.Fatalf("WorkspaceVCS = %q, want git", got)
	}
}

// A worktree's .git is a file, not a directory, and it is still a repository.
func TestWorkspaceVCSAcceptsWorktreeMarkerFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := WorkspaceVCS(root); got != "git" {
		t.Fatalf("WorkspaceVCS = %q, want git", got)
	}
}

func TestWorkspaceVCSReportsNoneOutsideARepository(t *testing.T) {
	// t.TempDir sits under the OS temp root, which is not itself a repository.
	if got := WorkspaceVCS(t.TempDir()); got != "" {
		t.Fatalf("WorkspaceVCS = %q, want none", got)
	}
}

func TestFormatSectionStatesVersionControlEitherWay(t *testing.T) {
	if got := FormatSection(nil, "test/os", "", "git", nil); !strings.Contains(got, "- Version control: git") {
		t.Fatalf("section = %q", got)
	}
	if got := FormatSection(nil, "test/os", "", "", nil); !strings.Contains(got, "- Version control: none") {
		t.Fatalf("section = %q", got)
	}
}

// Detection reads fileutil's declared table, so every store it names is one a
// workspace can be found under — not git alone.
func TestWorkspaceVCSNamesEveryDeclaredStore(t *testing.T) {
	for _, store := range fileutil.VCSStores() {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, store.Dir), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := WorkspaceVCS(root); got != store.Name {
			t.Fatalf("WorkspaceVCS under %s = %q, want %q", store.Dir, got, store.Name)
		}
	}
}

// A jj repo colocated with git keeps both stores. Probe order settles it, and
// the one the user drives is the one a plain git checkout does not have.
func TestWorkspaceVCSPrefersJJInAColocatedRepository(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".git", ".jj"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if got := WorkspaceVCS(root); got != "jj" {
		t.Fatalf("WorkspaceVCS = %q, want jj", got)
	}
}
