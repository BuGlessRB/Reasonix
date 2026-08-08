//go:build windows

package main

import (
	"fmt"
	"os"

	"reasonix/internal/winappid"
)

// applyWindowsTaskbarIdentity pins this process and any Reasonix taskbar
// shortcut to one explicit AppUserModelID before the first window registers.
// Explorer keys taskbar grouping on that ID, so the pinned launcher
// (Reasonix.exe / reasonix-launcher.exe) and the versioned desktop exe merge
// into a single button instead of two. It runs in both the primary and the
// remote-window process: both register windows, and the remote child must keep
// the same identity or it would split off its own taskbar button.
func applyWindowsTaskbarIdentity() {
	if err := winappid.SetProcessID(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: set AppUserModelID:", err)
	}
	if err := winappid.EnsureShortcutIDs(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: stamp AppUserModelID on pinned shortcuts:", err)
	}
}
