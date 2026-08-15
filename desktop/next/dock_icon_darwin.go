//go:build darwin && cgo

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void setReasonixDockIcon(const void *data, int length);
*/
import "C"

import (
	_ "embed"
	"unsafe"
)

// macOS reads the Dock icon out of the app bundle, and the shell also runs as a
// bare `go build ./next` binary with no bundle around it. Carrying the artwork
// is the counterpart to the committed .syso that brands the Windows build.
//
//go:embed appicon.icns
var dockIcon []byte

func applyDockIcon() {
	if len(dockIcon) == 0 {
		return
	}
	C.setReasonixDockIcon(unsafe.Pointer(&dockIcon[0]), C.int(len(dockIcon)))
}
