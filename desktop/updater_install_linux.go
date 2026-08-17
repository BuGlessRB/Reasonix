//go:build linux

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"reasonix/desktop/internal/update"
)

// dirIsWritable reports whether the process can create a temporary file in dir.
func dirIsWritable(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".reasonix-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// resolveExecutablePath returns the real path of the running binary.
func resolveExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// detectLinuxInstallProfile classifies the running Linux install.
//
//	deb      — dpkg owns the absolute executable path as reasonix-desktop, and the
//	           Polkit helper + pkexec are present for authorized upgrades.
//	portable — not dpkg-managed, install directory is writable (tarball flow).
//	manual   — system directory not writable, package ownership unclear, or
//	           authorization components missing.
func detectLinuxInstallProfile() installProfile {
	exe := resolveExecutablePath()
	if exe == "" {
		return installProfile{
			Mode:          installModeManual,
			CanSelfUpdate: false,
			ManualReason:  "cannot resolve the running executable path",
		}
	}

	if isDpkgOwnedReasonix(exe) {
		if linuxDebHelperReady() {
			return installProfile{
				Mode:          installModeDeb,
				CanSelfUpdate: true,
				RequiresElev:  true,
				ArtifactKind:  update.KindDeb,
			}
		}
		return installProfile{
			Mode:          installModeManual,
			CanSelfUpdate: false,
			ManualReason:  manualDebInstallHint() + " (Polkit helper or pkexec is missing)",
		}
	}

	dir := filepath.Dir(exe)
	if dirIsWritable(dir) {
		return installProfile{
			Mode:          installModePortable,
			CanSelfUpdate: true,
			ArtifactKind:  update.KindTarball,
		}
	}

	return installProfile{
		Mode:          installModeManual,
		CanSelfUpdate: false,
		ManualReason:  "this install is not writable and is not managed by the reasonix-desktop package; download the package from the download page",
	}
}

// isDpkgOwnedReasonix reports whether absolute path belongs to the reasonix-desktop
// package. Uses absolute dpkg-query and requires both package name and path match.
func isDpkgOwnedReasonix(absPath string) bool {
	return desktopLine().OwnsInstalledPath(absPath)
}
