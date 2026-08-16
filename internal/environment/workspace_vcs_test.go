package environment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
