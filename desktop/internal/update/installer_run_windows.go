//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
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
	cmd := exec.Command(path)
	// Detached: the installer must survive this process exiting, which is the
	// next thing the caller does.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update: start installer: %w", err)
	}
	return cmd.Process.Release()
}
