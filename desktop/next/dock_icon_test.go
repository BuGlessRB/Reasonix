package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The copy lives here because go:embed cannot reach out of the package
// directory. TestDarwinICNSUsesMacOSIconSafeArea guards the original, and this
// keeps the guarded artwork and the shipped one from drifting apart.
func TestDockIconMatchesTheDarwinBuildAsset(t *testing.T) {
	embedded, err := os.ReadFile("appicon.icns")
	if err != nil {
		t.Fatal(err)
	}
	authoritative, err := os.ReadFile(filepath.Join("..", "build", "darwin", "icon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, authoritative) {
		t.Fatalf("next/appicon.icns (%d bytes) differs from build/darwin/icon.icns (%d bytes); re-copy it", len(embedded), len(authoritative))
	}
}
