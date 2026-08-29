package control

import (
	"context"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/planmode"
)

// ATTACK: the compatibility endpoint approves by id, so it must not be able to
// approve "whatever is current". A boolean approve that ignored identity would
// be an escape hatch around every staleness rule the lifecycle enforces.
func TestLegacyApproveCannotAnswerADecisionItWasNotOffered(t *testing.T) {
	c := New(Options{Sink: event.Discard})
	defer c.Close()
	c.EnableInteractiveApproval()

	go c.requestApproval(context.Background(), approvalRequest{tool: planApprovalTool})
	first := waitForDecisions(t, c, 1)[0]
	if err := c.ResolvePlanDecision(first.ID, PlanDecisionRevisePlan); err != nil {
		t.Fatalf("revise: %v", err)
	}
	waitForDecisions(t, c, 0)

	// A second card, with an identity of its own.
	go c.requestApproval(context.Background(), approvalRequest{tool: planApprovalTool})
	second := waitForDecisions(t, c, 1)[0]
	if second.ID == first.ID {
		t.Fatalf("the second card reused identity %q", second.ID)
	}

	// The stale id, through the legacy path this time.
	c.Approve(first.ID, true, false, false)
	if got := c.Decisions(); len(got) != 1 || got[0].ID != second.ID {
		t.Fatalf("the legacy approve disturbed the open decision: %+v", got)
	}
	if got := c.PlanPhase(); got == planmode.Executing {
		t.Fatal("a stale legacy approve started execution")
	}
	c.Approve(second.ID, false, false, false)
	waitForDecisions(t, c, 0)
}

// Decision identities are minted per runtime, so two panes hand out the same
// strings. Nothing may treat one as globally unique: an id is only meaningful
// against the runtime that issued it, and this records that as a property
// rather than leaving it as an assumption a caller might not share.
func TestDecisionIdentitiesAreOnlyMeaningfulWithinTheirRuntime(t *testing.T) {
	newPane := func() (*Controller, Decision) {
		c := New(Options{Sink: event.Discard})
		c.EnableInteractiveApproval()
		go c.requestApproval(context.Background(), approvalRequest{tool: planApprovalTool})
		return c, waitForDecisions(t, c, 1)[0]
	}
	a, cardA := newPane()
	defer a.Close()
	b, cardB := newPane()
	defer b.Close()

	if cardA.ID != cardB.ID {
		t.Fatalf("panes minted %q and %q; this test is measuring the wrong thing", cardA.ID, cardB.ID)
	}

	// Answering one leaves the other exactly where it was — the runtimes share
	// no decision state, which is what makes routing the only thing that has to
	// be right.
	if err := a.ResolvePlanDecision(cardA.ID, PlanDecisionExitPlan); err != nil {
		t.Fatalf("exit on A: %v", err)
	}
	waitForDecisions(t, a, 0)
	if got := b.Decisions(); len(got) != 1 || got[0].ID != cardB.ID {
		t.Fatalf("answering A moved B: %+v", got)
	}
	if err := b.ResolvePlanDecision(cardB.ID, PlanDecisionExitPlan); err != nil {
		t.Fatalf("exit on B: %v", err)
	}
	waitForDecisions(t, b, 0)
}

// ATTACK: the specialized surface must not read "I am the Plan button" as "I may
// decide the current plan". Being the right kind of decision is not the same as
// being that decision, and the card the user clicked is the one that has to be
// answered.
func TestPlanSurfaceCannotAnswerADifferentPlanDecision(t *testing.T) {
	c := New(Options{Sink: event.Discard})
	defer c.Close()
	c.EnableInteractiveApproval()

	go c.requestApproval(context.Background(), approvalRequest{tool: planApprovalTool})
	stale := waitForDecisions(t, c, 1)[0]
	if err := c.ResolvePlanDecision(stale.ID, PlanDecisionRevisePlan); err != nil {
		t.Fatalf("revise: %v", err)
	}
	waitForDecisions(t, c, 0)

	go c.requestApproval(context.Background(), approvalRequest{tool: planApprovalTool})
	current := waitForDecisions(t, c, 1)[0]
	if current.ID == stale.ID {
		t.Fatalf("the second card reused identity %q", current.ID)
	}

	// Same surface, same kind, wrong instance.
	if err := c.ResolvePlanDecision(stale.ID, PlanDecisionStartExecution); err == nil {
		t.Fatal("a stale plan action answered a decision it was not offered")
	}
	if got := c.Decisions(); len(got) != 1 || got[0].ID != current.ID {
		t.Fatalf("the open decision was disturbed: %+v", got)
	}
	if got := c.PlanPhase(); got == planmode.Executing {
		t.Fatal("a stale plan action started execution")
	}
	if err := c.ResolvePlanDecision(current.ID, PlanDecisionExitPlan); err != nil {
		t.Fatalf("exit: %v", err)
	}
	waitForDecisions(t, c, 0)
}
