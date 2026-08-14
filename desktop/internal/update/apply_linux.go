//go:build linux

package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/installlayout"
)

// apply publishes the tarball into a new version directory and swaps the
// pointer last. Unlike Windows, Linux can replace files under a running
// process, so the install completes here and the caller relaunches.
func (v VersionedInstaller) apply(_ context.Context, c Cached) error {
	targz, err := os.ReadFile(c.Path)
	if err != nil {
		return err
	}
	release, err := ExtractReleaseUnit(targz)
	if err != nil {
		return err
	}
	root := v.Layout.Root
	if _, err := installlayout.ReadCurrent(root); err != nil {
		return fmt.Errorf("update: resolve active Linux layout: %w", err)
	}
	target := strings.TrimSpace(c.Version)
	if !strings.HasPrefix(target, "v") {
		target = "v" + target
	}
	if err := installlayout.ValidateVersionName(target); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(root, ".reasonix-linux-update-*")
	if err != nil {
		return fmt.Errorf("update: create Linux version staging: %w", err)
	}
	defer os.RemoveAll(staging)
	desktopPath := filepath.Join(staging, installlayout.DesktopBinaryName())
	cliPath := filepath.Join(staging, installlayout.CLIBinaryName())
	if err := os.WriteFile(desktopPath, release["reasonix-desktop"], 0o700); err != nil {
		return fmt.Errorf("update: stage Linux desktop: %w", err)
	}
	if err := os.WriteFile(cliPath, release["reasonix"], 0o700); err != nil {
		return fmt.Errorf("update: stage Linux CLI: %w", err)
	}
	if err := installlayout.ActivateVersion(installlayout.ActivationRequest{
		InstallRoot: root,
		Version:     target,
		RequestID:   "linux-" + target,
		Members: []installlayout.Member{
			{Name: installlayout.DesktopBinaryName(), Path: desktopPath, Mode: 0o700},
			{Name: installlayout.CLIBinaryName(), Path: cliPath, Mode: 0o700},
		},
		RequiredNames: []string{installlayout.DesktopBinaryName(), installlayout.CLIBinaryName()},
	}); err != nil {
		return fmt.Errorf("update: activate Linux version: %w", err)
	}
	_ = installlayout.RetainPreviousVersions(root, 0)
	return nil
}
