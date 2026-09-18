package identitylock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAliasPathsShareLocalIdentity(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(dir, "alias")
	if err := os.Symlink(realDir, aliasDir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	release, err := TryAcquire(filepath.Join(realDir, "state.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if aliasRelease, err := TryAcquire(filepath.Join(aliasDir, "state.lock")); !errors.Is(err, ErrHeld) {
		if aliasRelease != nil {
			aliasRelease()
		}
		t.Fatalf("alias acquire error = %v, want ErrHeld", err)
	}
}
