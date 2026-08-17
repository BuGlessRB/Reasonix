package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/desktop/internal/update"
)

func TestManifestRecordsEveryArtifactWithItsSignature(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITHUB_REPOSITORY", "esengine/DeepSeek-Reasonix")
	body := []byte("studio archive bytes")
	for _, name := range []string{
		"ReasonixStudio-windows-amd64-installer.exe",
		"ReasonixStudio-darwin-universal.dmg",
		"ReasonixStudio-linux-amd64.deb",
		"ReasonixStudio-windows-amd64.zip",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".minisig"), []byte("sig"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := run(dir, "v0.1.0", "studio-v0.1.0"); err != nil {
		t.Fatalf("run: %v", err)
	}

	var m update.Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("the manifest must parse as the type the updater reads: %v", err)
	}
	// The portable archive is a release asset but not an offer: downloads names
	// what installs itself, so a reader is not asked to choose between an
	// installer and a zip with no way to tell which one to take.
	if len(m.Downloads) != 3 {
		t.Fatalf("downloads = %d, want the installer, the disk image and the package", len(m.Downloads))
	}
	for name := range m.Downloads {
		if strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") {
			t.Errorf("downloads offers %q, which the reader has to place themselves", name)
		}
	}
	sum := sha256.Sum256(body)
	for name, a := range m.Downloads {
		if a.Sig != a.URL+".minisig" {
			t.Errorf("%s: sig %q does not point at the artifact's signature", name, a.Sig)
		}
		if a.Size != int64(len(body)) {
			t.Errorf("%s: size %d, want %d", name, a.Size, len(body))
		}
		if a.SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: sha256 %q does not match the bytes on disk", name, a.SHA256)
		}
	}
}

// A manifest that lists a platform asset is a claim that this build can install
// it, and the shared apply path cannot install Studio's archives yet. The claim
// has to arrive with the apply path that honours it, not before.
func TestManifestClaimsNoInstallableAsset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITHUB_REPOSITORY", "esengine/DeepSeek-Reasonix")
	if err := os.WriteFile(filepath.Join(dir, "ReasonixStudio-linux-amd64.tar.gz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(dir, "v0.1.0", "studio-v0.1.0"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var m update.Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Platforms) != 0 {
		t.Errorf("platforms = %v, want none until the apply path accepts Studio's layout", m.Platforms)
	}
	if m.DownloadPage == "" {
		t.Error("with no installable asset the download page is the only way forward, and it is empty")
	}
}

func TestEmptyDirIsAFailedRelease(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "esengine/DeepSeek-Reasonix")
	if err := run(t.TempDir(), "v0.1.0", "studio-v0.1.0"); err == nil {
		t.Fatal("a release with no artifacts must fail, not publish an empty manifest")
	}
}
