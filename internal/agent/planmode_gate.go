package agent

import (
	"encoding/json"
	"fmt"

	"reasonix/internal/planmode"
	"reasonix/internal/tool"
)

// The planning phase's admission decisions. planmode owns the rule; this is
// where the agent answers the two questions the rule is asked in terms of —
// can this call change state, and is this the planner's own research path.

func (a *Agent) planModeDecision(t tool.Tool, toolName string, readOnly bool, safety planmode.PlanSafety, args json.RawMessage) planmode.Decision {
	return (planmode.Policy{}).Decide(planmode.Call{
		Name:     toolName,
		ReadOnly: readOnly,
		Safety:   safety,
		Effect:   a.planPhaseEffect(t, toolName, readOnly),
		Args:     args,
	})
}

// planPhaseEffect answers the single question the phase gate asks: can this
// concrete call change state outside the session. readOnly is the per-call fact
// the plan already resolved — the tool's own answer, or the shell AST's for a
// concrete bash invocation — and the planner's research exemption is a host-known
// answer to the same question, so it belongs here and not at each gate.
func (a *Agent) planPhaseEffect(t tool.Tool, name string, readOnly bool) planmode.Effect {
	if readOnly || a.plannerTrustsMCP(t, name) {
		return planmode.EffectNone
	}
	return planmode.EffectSideEffect
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

// planPhaseGateForTarget decides the phase for the concrete target a call ends
// up at, which a proxy only names after resolution. It runs before Commit as
// well as before permission: a resolve-only state transition is still one, and
// Commit's own contract puts it after the host's checks.
func (a *Agent) planPhaseGateForTarget(plan *toolCallPlan) (toolOutcome, bool) {
	if !a.planMode.Load() {
		return toolOutcome{}, false
	}
	if plan.resolved.TargetName != "" {
		safety := planmode.PlanSafetyUnknown
		if c, ok := plan.execTool.(tool.PlanModeClassifier); ok {
			safety = planmode.PlanSafetyUnsafe
			if c.PlanModeSafe() {
				safety = planmode.PlanSafetySafe
			}
		}
		decision := a.planModeDecision(plan.execTool, plan.permName, plan.resolved.ReadOnly, safety, plan.permArgs)
		if decision.Blocked {
			return toolOutcome{output: decision.Message, blocked: true, errMsg: planPhaseBlockReason(decision)}, true
		}
	}
	if isMCPExecutionTarget(plan.execTool, plan.permName) && !a.plannerTrustsMCP(plan.execTool, plan.permName) &&
		(!plan.readOnly || !mcpServerAuthorized(plan.execTool) || mcpDestructiveHint(plan.execTool)) {
		reason := "writer/destructive target"
		if plan.readOnly && !mcpServerAuthorized(plan.execTool) {
			reason = "reader from an unauthorized server"
		}
		return toolOutcome{
			output:  fmt.Sprintf("blocked: MCP %s %q is unavailable during Plan mode; finish or exit Plan mode before requesting this call", reason, plan.permName),
			blocked: true,
			errMsg:  "blocked: MCP target is unavailable during planning",
		}, true
	}
	plan.planPhaseAdmitted = true
	return toolOutcome{}, false
}

// assertPlanPhaseAdmitted is the backstop for the barrier, not a second opinion
// on the call: it classifies nothing and re-decides nothing, it only refuses to
// execute what the phase gate never admitted. Reaching it means a pre-execution
// path skipped the gate, which is a host bug and says so.
func (a *Agent) assertPlanPhaseAdmitted(plan *toolCallPlan) (toolOutcome, bool) {
	if !a.planMode.Load() || plan.planPhaseAdmitted {
		return toolOutcome{}, false
	}
	return toolOutcome{
		output: "blocked: workflow invariant violation — a planning-phase call reached the execution boundary " +
			"without passing the plan gate. This is a host bug, not something to work around; report it.",
		blocked: true,
		errMsg:  "blocked: plan phase gate bypassed",
	}, true
}
