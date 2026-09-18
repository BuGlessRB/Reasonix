package cli

import (
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/i18n"
	"reasonix/internal/sandbox"
)

// showBoundary says what the agent may reach right now: the sandbox the
// configuration sets, and what this session was allowed on a prompt besides —
// which is written in no file, so a person reading the configuration is reading
// less than the agent may do. `revoke <rule>`, or `revoke all`, takes one back.
func (m *chatTUI) showBoundary(input string) {
	args := strings.Fields(strings.TrimSpace(input))
	if len(args) > 1 && args[1] == "revoke" {
		m.runRevokeCommand(strings.Join(args[2:], " "))
		return
	}
	m.showSandboxStatus()
	if granted := m.ctrl.PermissionRules().Granted; len(granted) > 0 {
		m.notice(strings.Join(granted, "\n") + "\n" + i18n.M.SlashRevokeHint)
	}
}

// showSandboxStatus displays the current sandbox configuration and whether
// the OS sandbox backend is available. It reads from the stored config so
// the user can inspect sandbox state without leaving the TUI (closes #3316).
func (m *chatTUI) showSandboxStatus() {
	if m.cfg == nil {
		m.notice("sandbox: config not loaded")
		return
	}
	bash := m.cfg.BashMode()
	network := m.cfg.Sandbox.Network
	available := sandbox.Available()
	roots := m.cfg.WriteRoots()

	var b strings.Builder
	b.WriteString("sandbox\n")
	b.WriteString("  phase 0  file-writer confinement\n")
	if len(roots) > 0 {
		fmt.Fprintf(&b, "    write_roots  %s\n", strings.Join(roots, ", "))
	}
	if m.cfg.Sandbox.WorkspaceRoot != "" {
		fmt.Fprintf(&b, "    workspace_root  %s\n", m.cfg.Sandbox.WorkspaceRoot)
	}
	if len(m.cfg.Sandbox.AllowWrite) > 0 {
		fmt.Fprintf(&b, "    allow_write  %s\n", strings.Join(m.cfg.Sandbox.AllowWrite, ", "))
	}
	b.WriteString("  phase 1  OS bash sandbox\n")
	fmt.Fprintf(&b, "    bash        %s", bash)
	if bash == "enforce" && !available {
		b.WriteString(" (unavailable: no OS sandbox on this host; bash execution is refused. " + sandbox.UnavailableRemediation() + ")")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "    network     %v\n", network)
	m.notice(b.String())
}

// runRevokeCommand lists what this session was allowed on a prompt, and takes
// one or all of them back. None of these are in the rules file, so without a
// way out the only one is ending the session.
func (m *chatTUI) runRevokeCommand(arg string) {
	granted := m.ctrl.PermissionRules().Granted
	if arg == "" {
		m.notice(i18n.M.SlashRevokeNone)
		return
	}
	rule := ""
	if arg != "all" {
		if !slices.Contains(granted, arg) {
			m.notice(fmt.Sprintf(i18n.M.SlashRevokeUnknown, arg))
			return
		}
		rule = arg
	}
	took := m.ctrl.RevokeSessionGrant(rule)
	m.notice(fmt.Sprintf(i18n.M.SlashRevokeDone, took, len(m.ctrl.PermissionRules().Granted)))
}
