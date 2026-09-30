//go:build windows

package builtin

import (
	"os"

	"golang.org/x/sys/windows"
)

// openedPath is the final path of an open handle, junctions and short names
// resolved: what was opened, whatever the string that named it.
func openedPath(f *os.File) (string, bool) {
	p, err := finalPathOfHandle(windows.Handle(f.Fd()))
	return p, err == nil
}
