//go:build windows

package pathidentity

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsIdentityKeyFoldsPerDirectory(t *testing.T) {
	caseSensitiveParent := filepath.Clean(`C:\Root`)
	identity := func(path string) string {
		key, err := windowsIdentityKeyBy(path, func(directory string) (bool, bool, error) {
			return !strings.EqualFold(filepath.Clean(directory), caseSensitiveParent), true, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	upper := identity(`C:\ROOT\Foo\LEAF.jsonl`)
	lower := identity(`c:\root\foo\leaf.jsonl`)
	if upper == lower {
		t.Fatalf("case-sensitive child names collapsed to %q", upper)
	}
	if got, want := upper, filepath.Clean(`c:\root\Foo\leaf.jsonl`); got != want {
		t.Fatalf("segment-aware identity = %q, want %q", got, want)
	}
}
