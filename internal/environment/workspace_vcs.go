package environment

import (
	"os"
	"path/filepath"
)

// WorkspaceVCS names the version control the workspace is under, or "" for
// none — a fact about the directory, not the tooling the probes already cover.
// It reads the filesystem instead of asking git, so it costs nothing at boot
// and cannot flap on a slow subprocess; the answer sits in the cached prefix.
func WorkspaceVCS(root string) string {
	dir := filepath.Clean(root)
	if dir == "" || dir == "." {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		dir = wd
	}
	for {
		// A worktree's .git is a file pointing at the real directory, so the
		// kind does not matter — only that the marker is there.
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return "git"
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
