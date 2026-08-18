// closing.go — the window stays up until its session is safely on disk.
package main

import (
	"context"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// A window that vanishes while its process is still saving has told the user the
// app is closed. Reopening then puts a second process on a session the first is
// still writing, and two writers on one transcript is what forks it into an
// endless run of recovery copies. So the close button starts the shutdown and
// the window stays up for it — nothing is faster, but nothing lies either.
type closePhase = int32

// A phase, not a pair of flags: "saving" and "done" are one question asked
// once, and two booleans would admit a done-but-not-saving state that means
// nothing here.
const (
	closeIdle closePhase = iota
	closeSaving
	closeDone
)

// closeState carries the phase. A type so the field reads as what it is at the
// declaration, rather than as an integer whose meaning lives elsewhere.
type closeState struct{ atomic.Int32 }

// quitWindow is runtime.Quit behind a variable, so the close sequence can be
// exercised without a Wails application behind the context.
var quitWindow = runtime.Quit

// beginClose answers Wails' "may the window close?" — false lets it go.
func (a *App) beginClose(ctx context.Context) (prevent bool) {
	if !a.closing.CompareAndSwap(closeIdle, closeSaving) {
		// Clicking again while it saves makes nothing faster, and must not take
		// the window down early either: only our own quit does that.
		return a.closing.Load() != closeDone
	}
	go a.finishClose(ctx)
	return true
}

// finishClose lands every pane and then asks the window to go, which comes back
// through beginClose with the phase that lets it.
func (a *App) finishClose(ctx context.Context) {
	a.hub.Shutdown()
	a.closing.Store(closeDone)
	quitWindow(ctx)
}
