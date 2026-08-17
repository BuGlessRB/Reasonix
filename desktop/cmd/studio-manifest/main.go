// Command studio-manifest writes Studio's latest.json from a directory of built
// artifacts.
//
// cmd/sign's manifest subcommand cannot be reused: it is the desktop line's, so
// it hardcodes desktop's download page and changelog URL, and its artifact
// matcher drops any windows file that is not -installer.exe.
//
// Every artifact lands in downloads and none in platforms. That is the honest
// state, not an oversight: the shared apply path names the desktop line's
// release members (see update.ExtractReleaseUnit) and Windows installs through
// NSIS, so Studio's portable archives have no install path yet. Filling
// platforms is what turns self-update on, and it may only happen together with
// an apply path that accepts this layout.
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
		Downloads:       map[string]update.Asset{},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, artifactPrefix) || strings.HasSuffix(name, ".minisig") {
			continue
		}
		size, sum, err := hashFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, name)
		m.Downloads[name] = update.Asset{URL: url, Sig: url + ".minisig", Size: size, SHA256: sum}
		fmt.Printf("download: %s (%d bytes)\n", name, size)
	}
	if len(m.Downloads) == 0 {
		return fmt.Errorf("studio-manifest: no %s* artifacts in %s", artifactPrefix, dir)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest.json"), append(b, '\n'), 0o644)
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
