//go:build !windows

package main

import (
	"context"

	"reasonix/internal/update"
)

// applyDownloaded installs what was downloaded, by staging it into the
// versioned layout and swapping. See install_apply_windows.go for why the
// platform split is a build tag rather than a GOOS check.
func (a *App) applyDownloaded(ctx context.Context, _, version string, dir string, layout update.Layout, cached update.Cached) error {
	inst := update.VersionedInstaller{Layout: layout, Staging: dir, Current: version, Line: studioLine()}
	return inst.Install(ctx, cached)
}
