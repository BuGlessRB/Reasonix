package cli

import (
	tea "charm.land/bubbletea/v2"

	"reasonix/internal/control"
)

// planDecisionFor names which transition the plan card answered. Three outcomes,
// three transitions: collapsing them into the approval boolean loses the
// difference between revising and leaving, and the kernel moves the lifecycle
// itself — setting plan mode alongside this would race its own transition.
func planDecisionFor(allow, exitPlan bool) control.PlanDecisionAction {
	switch {
	case allow:
		return control.PlanDecisionStartExecution
	case exitPlan:
		return control.PlanDecisionExitPlan
	default:
		return control.PlanDecisionRevisePlan
	}
}

// answerPlanCard routes the card's outcome to the kernel and reads the legacy
// flag back rather than setting it: the transition already moved the lifecycle.
func (m chatTUI) answerPlanCard(allow, exitPlan bool) (tea.Model, tea.Cmd) {
	_ = m.ctrl.ResolvePlanDecision(m.pendingApproval.ID, planDecisionFor(allow, exitPlan))
	m.planMode = m.ctrl.PlanMode()
	m.pendingApproval = nil
	return m, nil
}
