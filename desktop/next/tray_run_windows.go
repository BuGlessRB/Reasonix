//go:build windows

package main

import (
	"runtime"

	"fyne.io/systray"
)

// runTray gives the icon its own OS thread. GetMessage only ever sees the
// messages of the thread that owns the window, and RunWithExternalLoop — which
// reads like the embedding path — pumps from a goroutine nobody pinned. The
// icon drew, because Shell_NotifyIcon does that, and then answered nothing.
func runTray(ready, exit func()) (stop func()) {
	go func() {
		runtime.LockOSThread()
		systray.Run(ready, exit)
	}()
	return systray.Quit
}
