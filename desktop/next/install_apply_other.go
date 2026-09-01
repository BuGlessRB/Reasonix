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
	// This shell is its own application: the window and the binary being
	// replaced are one process. Saying so is what the other shell will not be
	// able to say, which is why it is stated here rather than assumed there.
	app, err := update.LocalApplication(layout)
	if err != nil {
		return err
	}
	inst := update.VersionedInstaller{Layout: layout, Staging: dir, Current: version, Line: studioLine(), App: app}
	return inst.Install(ctx, cached)
}
