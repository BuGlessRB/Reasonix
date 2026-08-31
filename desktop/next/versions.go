package main

import "reasonix/internal/update"

// version is stamped by the build (-X main.version=…); an unstamped dev build
// says so rather than pretending to be a release.
var version = "dev"

// install is what this shell knows about itself: which build runs and where it
// lives. The kernel cannot work either out for a shell that is not the process
// asking, so each one states its own.
func (a *App) install() update.Install {
	return update.Install{Version: version, Layout: update.Here(studioLine())}
}
