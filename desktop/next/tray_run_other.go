//go:build !windows && !darwin

package main

import "fyne.io/systray"

// runTray keeps the documented embedding path here. Giving the icon a thread of
// its own — the Windows fix — is not it: on the platforms left here the icon
// rides the caller's goroutine. macOS needs the main thread and has its own
// file.
func runTray(ready, exit func()) (stop func()) {
	start, end := systray.RunWithExternalLoop(ready, exit)
	start()
	return end
}
