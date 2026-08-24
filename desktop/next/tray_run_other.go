//go:build !windows

package main

import "fyne.io/systray"

// runTray keeps the documented embedding path here. macOS puts its status item
// on the main thread and Wails already owns that one, so handing the icon a
// thread of its own — the Windows fix — would be a crash rather than a dead
// icon. Neither platform has been run from the machine this was written on;
// this is the shape to check first if the icon misbehaves there.
func runTray(ready, exit func()) (stop func()) {
	start, end := systray.RunWithExternalLoop(ready, exit)
	start()
	return end
}
