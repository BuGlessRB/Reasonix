package control

import (
	"sort"

	"reasonix/internal/event"
)

// Decisions are projections of host-owned state, not state themselves. Acting
// on one routes back to its owning subsystem and is accepted only if the
// decision identity is still current. Nothing in the kernel reads this view.

// DecisionKind names which subsystem owns a decision, and therefore which call
// answers it. A frontend may render them in one list; the host does not merge
// them, because "approve a workflow transition" and "tell me which option you
// want" are answered by different code with different rules.
type DecisionKind string

const (
	// DecisionPlanApproval is answered by ResolvePlanDecision.
	DecisionPlanApproval DecisionKind = "plan_approval"
	// DecisionAsk is answered by AnswerQuestion.
	DecisionAsk DecisionKind = "ask"
)

// Decision is one thing waiting on the user. ID is the identity its owner
// already issued; a frontend sends it back untouched and the owner resolves it,
// so a card answered after its subject moved on resolves nothing. The host does
// not expose the phase or epoch behind that identity — authority arithmetic is
// the kernel's, not the UI's.
type Decision struct {
	ID        string              `json:"id"`
	Kind      DecisionKind        `json:"kind"`
	Questions []event.AskQuestion `json:"questions,omitempty"`
}

// Decisions snapshots what waits on the user, derived from the owning
// subsystems each time it is asked for and never the thing they derive from.
// The host serialises prompts, so today it holds at most one; the shape does
// not assume that, because a frontend would then depend on it.
func (c *Controller) Decisions() []Decision {
	if c == nil {
		return nil
	}
	return c.approval.projectDecisions()
}

func (a *approvalManager) projectDecisions() []Decision {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Decision, 0, len(a.asks)+1)
	for id, pending := range a.approvals {
		if pending.tool != planApprovalTool {
			continue // ordinary tool permission has its own owner and its own card
		}
		out = append(out, Decision{ID: id, Kind: DecisionPlanApproval})
	}
	for id, pending := range a.asks {
		// A queued ask is included: it is already blocking the turn, and this is
		// a snapshot a frontend pulls rather than an event it might replay. The
		// queued flag exists to stop replaying a question nobody ever saw.
		out = append(out, Decision{ID: id, Kind: DecisionAsk, Questions: pending.questions})
	}
	sortDecisions(out)
	return out
}

// sortDecisions keeps the projection stable across repeated snapshots: Go map
// order is random, and a list that reshuffles every poll makes a frontend
// rebuild cards that never changed.
func sortDecisions(d []Decision) {
	sort.Slice(d, func(i, j int) bool { return d[i].ID < d[j].ID })
}
