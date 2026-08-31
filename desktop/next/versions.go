package main

import (
	"context"

	"reasonix/internal/update"
)

// version is stamped by the build (-X main.version=…); an unstamped dev build
// says so rather than pretending to be a release.
var version = "dev"

// install is what this shell knows about itself: which build runs and where it
// lives. The kernel cannot work either out for a shell that is not the process
// asking, so each one states its own.
func (a *App) install() update.Install {
	return update.Install{Version: version, Layout: update.Here(studioLine())}
}

// Versions reads the rollback catalog through the kernel, so this shell holds
// no copy of what "newer", "pinned" or "latest" mean.
func (a *App) Versions() update.VersionHub {
	return update.Hub(context.Background(), a.install())
}

// PinVersion holds this machine on a release, or releases the hold when the
// version is empty.
func (a *App) PinVersion(pin string) error { return update.Pin(pin) }
