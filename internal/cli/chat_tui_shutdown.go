// chat_tui_shutdown.go — the one exit every quit gesture and signal funnels into.
package cli

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/i18n"
)

// shutdownAndQuit persists what the controller holds beyond the last snapshot
// and leaves. It takes the shutdown variant rather than a plain Snapshot: when
// another instance holds this session's file lock the plain one only returns
// the error, and the transcript in memory goes with the process.
func (m chatTUI) shutdownAndQuit() (tea.Model, tea.Cmd) {
	if m.ctrl != nil {
		m.shutdownErr = m.ctrl.SnapshotForShutdown()
		m.followSessionLease()
	}
	return m, tea.Quit
}

// reportShutdownFailure prints what the final save could not do. It runs once
// the alt-screen is gone, which is why the TUI cannot show it — and staying
// silent would hide that this session's transcript never reached disk.
func reportShutdownFailure(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
}
