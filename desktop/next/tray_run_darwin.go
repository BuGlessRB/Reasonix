//go:build darwin

package main

/*
void rxTrayStartOnMain(void);
*/
import "C"

import "fyne.io/systray"

// systray's nativeStart builds the NSStatusItem inline, and AppKit only lets a
// window be instantiated on the main thread — which a Wails lifecycle hook is
// not. Every other systray call already hops there itself; this one does not,
// so the hop happens here. Async is safe: ready() ends by replaying the last
// state, and a setter that lands before the delegate exists is a nil no-op.
var trayStart func()

func runTray(ready, exit func()) (stop func()) {
	start, end := systray.RunWithExternalLoop(ready, exit)
	trayStart = start
	C.rxTrayStartOnMain()
	return end
}

//export rxTrayStart
func rxTrayStart() {
	if trayStart != nil {
		trayStart()
	}
}
