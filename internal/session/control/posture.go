package control

import "reasonix/internal/contract/config"

// ToolApprovalReadOnly refuses every call that is not a read, whatever the
// rules allow; no approval can talk a write past it.
const ToolApprovalReadOnly = "readOnly"

// PostureEvidence is what a terminal session's default posture is decided
// from. WritesConfined is the bash sandbox's Integrity claim for this build,
// as its backend reports it; Home is where the person recorded folder trust.
type PostureEvidence struct {
	WritesConfined bool
	Home           string
}

// DefaultApprovalMode is the posture a session opens in when nobody named one:
// writes inside the workspace run without asking only where an OS sandbox
// confines them and the person trusted this folder. Everywhere else — no
// backend, bash off, Windows, an undecided or declined folder — it asks.
func DefaultApprovalMode(writesConfined bool, trust config.WorkspaceTrust) string {
	if writesConfined && trust == config.WorkspaceTrusted {
		return ToolApprovalAuto
	}
	return ToolApprovalAsk
}

// DefaultApprovalMode reads the trust record now, so a folder trusted after
// this controller was built counts.
func (c *Controller) DefaultApprovalMode() string {
	return DefaultApprovalMode(c.posture.WritesConfined, c.WorkspaceTrust())
}

// WorkspaceTrust is the person's recorded decision about this workspace; an
// unreadable record is undecided, which costs a prompt and grants nothing.
func (c *Controller) WorkspaceTrust() config.WorkspaceTrust {
	trust, _ := config.NewProjectGrantStore(c.posture.Home).Trust(c.WorkspaceRoot())
	return trust
}

// WritesConfined reports this build's bash Integrity claim.
func (c *Controller) WritesConfined() bool { return c.posture.WritesConfined }

// SetWorkspaceTrust records the person's decision about this workspace under
// the same home DefaultApprovalMode reads it from.
func (c *Controller) SetWorkspaceTrust(trust config.WorkspaceTrust) error {
	return config.NewProjectGrantStore(c.posture.Home).SetTrust(c.WorkspaceRoot(), trust)
}
