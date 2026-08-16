package boot

import (
	"reasonix/internal/agent"
	"reasonix/internal/tool"
)

// addTurnExitTools registers the two ways a turn ends on the model's terms
// rather than the host's. Both stay registered in every turn: a tool that comes
// and goes changes the cached prompt prefix, and neither condition — a decision
// only the user can make, a task that cannot be done as specified — is one the
// host can recognize before the model does.
func addTurnExitTools(reg *tool.Registry) {
	// ask reaches the user through the Asker on the call context, which
	// interactive frontends wire to the controller (EnableInteractiveApproval);
	// a headless run has none, so ask resolves to "decide for yourself".
	reg.Add(agent.NewAskTool())
	reg.Add(agent.NewConcludeBlockedTool())
}
