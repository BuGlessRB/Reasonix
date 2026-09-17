package bot

import "reasonix/internal/control"

// stubBotController answers the calls the gateway makes on every session, so a
// fake states only what its test is about. Everything else reaches the nil
// botController and panics: a fake missing a method the gateway starts calling
// fails at once on every platform, instead of being recovered somewhere.
type stubBotController struct {
	botController
}

func (stubBotController) SetToolApprovalMode(string)           {}
func (stubBotController) WorkspaceRoot() string                { return "" }
func (stubBotController) RuntimeStatus() control.RuntimeStatus { return control.RuntimeStatus{} }
