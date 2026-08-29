package agent

import (
	"encoding/json"

	"reasonix/internal/planmode"
	"reasonix/internal/tool"
)

// The planning phase's admission decisions. planmode owns the rule; this is
// where the agent answers the two questions the rule is asked in terms of —
// can this call change state, and is this the planner's own research path.

func (a *Agent) planModeDecision(toolName string, readOnly bool, safety planmode.PlanSafety, args json.RawMessage) planmode.Decision {
	// readOnly is the per-call fact the plan already resolved: the tool's own
	// answer for ordinary tools, the shell AST's for a concrete bash invocation.
	effect := planmode.EffectSideEffect
	if readOnly {
		effect = planmode.EffectNone
	}
	return (planmode.Policy{}).Decide(planmode.Call{
		Name:     toolName,
		ReadOnly: readOnly,
		Safety:   safety,
		Effect:   effect,
		Args:     args,
	})
}

// plannerTrustsMCP reports the two-model planner's standing exemption: an
// authorized, non-destructive MCP target is how the planner researches, so it
// runs during planning even though a writer-capable schema cannot say so.
func (a *Agent) plannerTrustsMCP(t tool.Tool, name string) bool {
	return a.role.plannerMCPExecution && isMCPExecutionTarget(t, name) &&
		mcpServerAuthorized(t) && !mcpDestructiveHint(t)
}

// planPhaseBlockReason keeps the three ways a call fails the phase apart where
// the host can count them: unclassified is debt a classifier can close, the
// others are the barrier working.
func planPhaseBlockReason(d planmode.Decision) string {
	switch d.Reason {
	case planmode.BlockUnclassified:
		return "blocked: effect not classified during planning"
	case planmode.BlockSideEffect:
		return "blocked: side effect during planning"
	default:
		return "blocked: tool is unavailable during planning"
	}
}
