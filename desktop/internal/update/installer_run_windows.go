//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// RunInstaller starts a downloaded installer and returns once it is running.
// The caller then exits: Windows will not replace an executable a live process
// holds open, and the installer waits for that handle to clear. Nothing is
// staged or swapped here — for a line that installs rather than publishing
// version directories, the installer is the helper.
func (l Line) RunInstaller(path string) error {
	if path == "" {
		return fmt.Errorf("update: no installer to run")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("update: installer is unreadable: %w", err)
	}
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("update: start installer: %w", err)
	}
	target, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("update: start installer: %w", err)
	}
	// CreateProcess refuses an image whose manifest requests admin, so
	// exec.Command can never start this one. ShellExecute raises the consent
	// prompt, and what it starts already outlives this process.
	if err := windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return runInstallerStartError(err)
	}
	return nil
}

// runInstallerStartError keeps the installer path out of the text: it reaches
// the user, and the path names their account.
func runInstallerStartError(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_CANCELLED):
		return fmt.Errorf("update: the administrator prompt was dismissed; the installed version is unchanged")
	case errors.Is(err, windows.ERROR_FILE_NOT_FOUND), errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
		return fmt.Errorf("update: the downloaded installer was gone before it started; security software may have quarantined it")
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("update: Windows or security software denied starting the installer")
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return fmt.Errorf("update: start installer: Windows error %w (%s)", errno, errno.Error())
	}
	return fmt.Errorf("update: start installer failed")
}
