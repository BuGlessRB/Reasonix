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
// exercised without a Wails application behind the context. hideWindow is the
// same treatment for the branch that keeps the process alive.
var (
	quitWindow = runtime.Quit
	hideWindow = runtime.WindowHide
)

// beginClose answers Wails' "may the window close?" — false lets it go.
func (a *App) beginClose(ctx context.Context) (prevent bool) {
	// Where an icon can bring it back, the close button hides. Nothing else
	// changes: the session stays open because it is still this process's, and
	// the icon is what keeps that visible rather than a ghost.
	if a.background.Load() && a.closing.Load() == closeIdle {
		hideWindow(ctx)
		return true
	}
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

// showWindow brings a hidden window back. Unminimise first: a window that was
// minimised and then hidden needs both, and asking for one it does not need is
// free.
func (a *App) showWindow() {
	ctx := a.context()
	if ctx == nil {
		return
	}
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
}

// quitFromTray is the close the window's own button takes when nothing is
// backgrounded: land every pane, then let the window go. It is the same
// sequence rather than a second one, because the reason that sequence exists —
// two processes writing one transcript — does not care which control asked.
func (a *App) quitFromTray() {
	ctx := a.context()
	if ctx == nil {
		return
	}
	if !a.closing.CompareAndSwap(closeIdle, closeSaving) {
		return
	}
	go a.finishClose(ctx)
}

func (a *App) context() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx
}
