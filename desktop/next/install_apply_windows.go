//go:build windows

package main

import (
	"context"

	"reasonix/desktop/internal/update"
)

// applyDownloaded installs what was downloaded. Windows publishes an installer,
// not an archive: it replaces the install itself, so nothing here stages or
// swaps, and going back is running an older installer. A build tag rather than
// a GOOS check because the other half is a stub that always errors — the
// runtime branch made every other platform compile a call that could only fail.
func (a *App) applyDownloaded(_ context.Context, target, _ string, dir string, layout update.Layout, cached update.Cached) error {
	_ = dir
	return studioLine().RunInstaller(cached.Path)
}
