//go:build windows

package builtin

import (
	"strings"

	"golang.org/x/sys/windows"
)

const pathSeparators = `/\`

// resolveExisting asks the operating system for the final path of an existing
// file or directory through an open handle. That is the only form that follows
// junctions and mount points (EvalSymlinks does not), and it also settles 8.3
// short names, the case the file has on disk, and the \\?\ and UNC spellings,
// so every Windows form of a path reaches one comparison.
func resolveExisting(p string) (string, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	return finalPathOfHandle(h)
}

func finalPathOfHandle(h windows.Handle) (string, error) {
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) <= len(buf) {
			return trimExtendedPrefix(windows.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n)
	}
}

func trimExtendedPrefix(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`):
		return p[len(`\\?\`):]
	}
	return p
}
