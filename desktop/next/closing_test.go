package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/serve"
)

// The close button must not take the window down while the session is still
// being written: a window that is gone reads as an app that has closed, and
// reopening on top of a process that is still saving is what puts two writers on
// one transcript.
func TestTheWindowOutlivesItsCloseButton(t *testing.T) {
	quit := make(chan struct{}, 1)
	restore := quitWindow
	quitWindow = func(context.Context) { quit <- struct{}{} }
	t.Cleanup(func() { quitWindow = restore })

	a := &App{hub: serve.NewHub(serve.HubOptions{})}
	ctx := context.Background()

	if !a.beginClose(ctx) {
		t.Fatal("the first click let the window go before anything was saved")
	}
	select {
	case <-quit:
	case <-time.After(2 * time.Second):
		t.Fatal("the shutdown never asked the window to quit")
	}
	// The quit it just asked for arrives as another close: this one may pass.
	if a.beginClose(ctx) {
		t.Fatal("the window refused the quit its own shutdown asked for")
	}
}

// Clicking again while it saves is not a way to skip the saving.
func TestClickingCloseAgainDoesNotCutTheSaveShort(t *testing.T) {
	a := &App{hub: serve.NewHub(serve.HubOptions{})}
	a.closing.Store(closeSaving)
	for range 3 {
		if !a.beginClose(context.Background()) {
			t.Fatal("a click while the session was still being written took the window down")
		}
	}
	if a.closing.Load() != closeSaving {
		t.Error("clicking again moved the shutdown along; only the shutdown itself may")
	}
}

// Backgrounding is what a status icon buys: the window goes away, the session
// does not. Saving here would be the opposite of the point — nothing is being
// closed, so nothing has to be landed.
func TestBackgroundingHidesTheWindowWithoutClosingTheSession(t *testing.T) {
	hidden := make(chan struct{}, 1)
	restoreHide, restoreQuit := hideWindow, quitWindow
	hideWindow = func(context.Context) { hidden <- struct{}{} }
	quitWindow = func(context.Context) { t.Error("backgrounding asked the window to quit") }
	t.Cleanup(func() { hideWindow, quitWindow = restoreHide, restoreQuit })

	a := &App{hub: serve.NewHub(serve.HubOptions{})}
	a.background.Store(true)
	if !a.beginClose(context.Background()) {
		t.Fatal("the window closed instead of stepping aside")
	}
	select {
	case <-hidden:
	case <-time.After(2 * time.Second):
		t.Fatal("the window neither closed nor hid")
	}
	if a.closing.Load() != closeIdle {
		t.Fatal("hiding started a shutdown; nothing is being closed")
	}
}

// Quitting from the icon is the same sequence the close button runs when
// nothing is backgrounded — the reason that sequence exists does not care which
// control asked for it.
func TestQuittingFromTheIconStillLandsTheSession(t *testing.T) {
	quit := make(chan struct{}, 1)
	restoreHide, restoreQuit := hideWindow, quitWindow
	hideWindow = func(context.Context) { t.Error("the quit hid the window instead") }
	quitWindow = func(context.Context) { quit <- struct{}{} }
	t.Cleanup(func() { hideWindow, quitWindow = restoreHide, restoreQuit })

	a := &App{hub: serve.NewHub(serve.HubOptions{})}
	a.background.Store(true)
	a.mu.Lock()
	a.ctx = context.Background()
	a.mu.Unlock()

	a.quitFromTray()
	select {
	case <-quit:
	case <-time.After(2 * time.Second):
		t.Fatal("the quit never landed the session")
	}
	if a.closing.Load() != closeDone {
		t.Fatalf("phase = %d, want the shutdown finished", a.closing.Load())
	}
}
