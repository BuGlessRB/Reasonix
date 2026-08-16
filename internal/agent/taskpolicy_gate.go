package agent

// taskPolicyToolGate enforces the turn's plan-mode read-only boundary for both
// direct tools and use_capability-resolved targets before any mutation. Limits
// the user asked for in prose are the permission system's to enforce, on the
// concrete command, not this gate's to infer from wording.
func (a *Agent) taskPolicyToolGate(plan *toolCallPlan) (toolOutcome, bool) {
	if !a.turn.policySet {
		return toolOutcome{}, false
	}
	if plan.mutates && !a.turn.policy.AllowsMutation() {
		return policyBlock("plan mode is read-only; leave plan mode before changing the workspace", "task policy forbids mutation")
	}
	return toolOutcome{}, false
}

func policyBlock(output, reason string) (toolOutcome, bool) {
	return toolOutcome{output: "blocked: " + output, blocked: true, errMsg: "blocked: " + reason}, true
}
