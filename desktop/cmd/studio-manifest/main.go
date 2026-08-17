// Command studio-manifest writes Studio's latest.json from a directory of built
// artifacts. cmd/sign's manifest subcommand is the desktop line's: it hardcodes
// desktop's download page and drops any windows file that is not -installer.exe.
//
// Every artifact lands in downloads, none in platforms — the shared apply path
// names the desktop line's release members (update.ExtractReleaseUnit) and
// Windows installs through NSIS, so Studio has no install path yet. Filling
// platforms turns self-update on, and must land with that path.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"reasonix/desktop/internal/update"
)

const artifactPrefix = "ReasonixStudio-"

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: studio-manifest <dir> <version> <tag>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir, version, tag string) error {
	repo := os.Getenv("GITHUB_REPOSITORY")
	if strings.TrimSpace(repo) == "" {
		return fmt.Errorf("studio-manifest: GITHUB_REPOSITORY is unset, so asset URLs cannot be built")
	}
	page := fmt.Sprintf("https://github.com/%s/releases/tag/%s", repo, tag)
	m := update.Manifest{
		Version:         version,
		DownloadPage:    page,
		ReleaseNotesURL: page,
		Platforms:       map[string]update.Asset{},
		NativePackages:  map[string]update.Asset{},
		Downloads:       map[string]update.Asset{},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	seen := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, artifactPrefix) || strings.HasSuffix(name, ".minisig") {
			continue
		}
		seen++
		size, sum, err := hashFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, name)
		asset := update.Asset{URL: url, Sig: url + ".minisig", Size: size, SHA256: sum}
		// Downloads is what a person is offered, so it lists what installs
		// itself. A portable archive stays a release asset, but offering it
		// beside the installer only asks the reader to choose blind.
		if installable(name) {
			m.Downloads[name] = asset
			fmt.Printf("download: %s (%d bytes)\n", name, size)
		}
		// A .deb installs through dpkg, which carries a binary and the SPA tree
		// alike — so it is the one artifact Studio can self-update from today.
		// Platforms stays empty until the shared apply path can publish a tree.
		if key, ok := nativePackageKey(name); ok {
			m.NativePackages[key] = asset
			fmt.Printf("native package: %s -> %s\n", name, key)
		}
	}
	if seen == 0 {
		return fmt.Errorf("studio-manifest: no %s* artifacts in %s", artifactPrefix, dir)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest.json"), append(b, '\n'), 0o644)
}

// installable reports whether an artifact installs itself rather than expecting
// the reader to place it: a disk image to drag from, an installer to run, a
// package for dpkg.
func installable(name string) bool {
	return strings.HasSuffix(name, ".dmg") ||
		strings.HasSuffix(name, "-installer.exe") ||
		strings.HasSuffix(name, ".deb")
}

// nativePackageKey maps a .deb artifact name to the platform whose dpkg install
// it upgrades. Artifacts are named ReasonixStudio-linux-<arch>.deb, and only a
// package channel belongs here — a tarball is a download, not an install.
func nativePackageKey(name string) (string, bool) {
	base, ok := strings.CutSuffix(name, ".deb")
	if !ok {
		return "", false
	}
	arch, ok := strings.CutPrefix(base, artifactPrefix+"linux-")
	if !ok || arch == "" {
		return "", false
	}
	return update.PlatformKey("linux", arch), true
}

func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}
