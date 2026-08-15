package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// The case the panel got wrong: a file created and then removed leaves two tool
// events behind and nothing on disk, so it must not stay pending.
func TestCreatedThenDeletedIsNotAChange(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")

	scratch := filepath.Join(dir, "scratch.go")
	if err := os.WriteFile(scratch, []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changes, ok, err := Status(context.Background(), dir)
	if err != nil || !ok {
		t.Fatalf("Status = %v ok=%v", err, ok)
	}
	if len(changes) != 1 || changes[0].Path != "scratch.go" || !changes[0].Added() {
		t.Fatalf("after create: %+v, want one added scratch.go", changes)
	}

	if err := os.Remove(scratch); err != nil {
		t.Fatal(err)
	}
	changes, _, err = Status(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("after delete: %+v, want no pending change", changes)
	}

	// And a tracked file removed by a shell command is a real pending deletion.
	if err := os.Remove(filepath.Join(dir, "keep.go")); err != nil {
		t.Fatal(err)
	}
	changes, _, err = Status(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || !changes[0].Deleted() {
		t.Fatalf("after removing a tracked file: %+v, want one deletion", changes)
	}
}

func TestNonRepoReportsFallbackNotFailure(t *testing.T) {
	if _, ok, err := Status(context.Background(), t.TempDir()); ok || err != nil {
		t.Fatalf("Status(non-repo) = ok=%v err=%v, want ok=false with no error", ok, err)
	}
}
