package main

import (
	"context"
	"os"
	goruntime "runtime"

	"reasonix/internal/update"
)

// The three acts of a handover, which this shell performs as one process
// because it is its own application. The kernel decides when; these say how,
// and they are separate because they are not one act everywhere.

// PrepareForUpdate releases what would keep this build's own files from being
// replaced. The exit that has to follow belongs to EndApplication: what
// releases the application and what ends it are not the same act.
func (a *App) PrepareForUpdate(context.Context) error {
	if a.hub != nil {
		a.hub.Shutdown()
	}
	return nil
}

// RelaunchAfterUpdate starts what replaced this build, where that is this
// process's half of it. Windows and macOS already have a helper waiting for the
// exit; Linux replaced the files in place, so nobody else is waiting.
func (a *App) RelaunchAfterUpdate(context.Context) error {
	if goruntime.GOOS != "linux" {
		return nil
	}
	return update.Here(studioLine()).Relaunch()
}

// EndApplication ends this process, which for this shell is the application.
func (a *App) EndApplication(context.Context) {
	os.Exit(0)
}
