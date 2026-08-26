// app_identity.go — the identity Windows groups this window's taskbar button
// and routes its notifications by.
package main

import (
	"fmt"
	"io"
	"os"

	"reasonix/internal/appidentity"
	"reasonix/internal/installlayout"
)

// applyAppIdentity stamps this process with the AppUserModelID and makes the
// installed shortcuts carry the same one. Windows routes a toast through the
// Start Menu shortcut that matches, so a shortcut left on an older identity —
// an upgrade, a repaired install — silently swallows every notification. Both
// calls are no-ops off Windows.
func applyAppIdentity(logs io.Writer) {
	if err := appidentity.ApplyToCurrentProcess(); err != nil {
		fmt.Fprintln(logs, "reasonix-next: apply app identity:", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return
	}
	installRoot, err := installlayout.ResolveInstallRoot(executable)
	if err != nil {
		return
	}
	if err := appidentity.RepairOwnedShortcuts(installRoot); err != nil {
		fmt.Fprintln(logs, "reasonix-next: repair shortcut identity:", err)
	}
}
