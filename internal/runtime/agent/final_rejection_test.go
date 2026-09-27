package agent

import (
	"reasonix/internal/state/sessionstore"
	"strings"
	"testing"

	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/plancontract"
	"reasonix/internal/safety/evidence"
)

func rejectionAgent(t *testing.T, plan *plancontract.Plan) *Agent {
	t.Helper()
	a := New(nil, tool.NewRegistry(), sessionstore.NewSession(""), Options{}, nil)
	a.resetTurnEvidence()
	a.turn.turnInput = "fix the retry race"
	a.SetPlanContract(plan)
	return a
}

func checkedPlan() plancontract.Plan {
	plan := criterionPlan()
	plan.Steps[0].Verification = []plancontract.Verification{{Command: "go test ./pay/"}}
	return plan.Normalize()
}

// The gate has to name what is missing: a criterion whose step names a check
// that has not passed since the change, with the check that settles it.
func TestUnprovenPlanCriterionBlocksTheFinalAnswer(t *testing.T) {
	plan := checkedPlan()
	a := rejectionAgent(t, &plan)
	a.task.ledger.Record(evidence.Receipt{ToolName: "edit_file", Mutation: true, Write: true, Success: true, Paths: []string{"pay.go"}})

	joined := strings.Join(a.outstandingPlanCriteria(), "; ")
	if !strings.Contains(joined, "retries no longer double-charge") || !strings.Contains(joined, "run go test ./pay/") {
		t.Fatalf("outstanding = %q, want the criterion and the check that settles it", joined)
	}
}

// A citation is the model's word: it does not settle a criterion whose step
// names a check, and a criterion no command checks is not held at all.
func TestACitationSettlesNoPlanCriterion(t *testing.T) {
	plan := checkedPlan()
	a := rejectionAgent(t, &plan)
	a.task.ledger.Record(evidence.Receipt{ToolName: "edit_file", Mutation: true, Write: true, Success: true, Paths: []string{"pay.go"}})
	for _, c := range plan.Steps[0].Acceptance {
		a.task.ledger.Record(completeStepReceipt(t, c.ID, "manual", ""))
	}
	if len(a.outstandingPlanCriteria()) == 0 {
		t.Fatal("manual citations settled criteria whose step names a check")
	}
	unchecked := criterionPlan()
	b := rejectionAgent(t, &unchecked)
	b.task.ledger.Record(evidence.Receipt{ToolName: "edit_file", Mutation: true, Write: true, Success: true, Paths: []string{"pay.go"}})
	if outstanding := b.outstandingPlanCriteria(); len(outstanding) != 0 {
		t.Fatalf("outstanding = %v, want criteria no command checks left to the receipt, not the gate", outstanding)
	}
}

// Once the step's check passes after the change the gate lets go, and a later
// change reopens it: a pass from before the last change proves nothing now.
func TestPlanCheckMustPassAfterTheLatestChange(t *testing.T) {
	plan := checkedPlan()
	a := rejectionAgent(t, &plan)
	edit := evidence.Receipt{ToolName: "edit_file", Mutation: true, Write: true, Success: true, Paths: []string{"pay.go"}}
	a.task.ledger.Record(edit)
	zero := 0
	a.task.ledger.Record(evidence.Receipt{ToolName: "bash", Command: "go test ./pay/", Success: true, ExitCode: &zero})
	if outstanding := a.outstandingPlanCriteria(); len(outstanding) != 0 {
		t.Fatalf("outstanding = %v, want none once the step's check passed after the change", outstanding)
	}
	a.task.ledger.Record(edit)
	if len(a.outstandingPlanCriteria()) == 0 {
		t.Fatal("a change after the check must reopen the criteria")
	}
}

// An unplanned turn must not inherit a contract it never agreed to.
func TestUnplannedTurnHasNoOutstandingCriteria(t *testing.T) {
	a := rejectionAgent(t, nil)
	a.task.ledger.Record(evidence.Receipt{ToolName: "edit_file", Mutation: true, Write: true, Success: true, Paths: []string{"pay.go"}})
	if outstanding := a.outstandingPlanCriteria(); len(outstanding) != 0 {
		t.Fatalf("outstanding = %v, want none without an approved plan", outstanding)
	}
}
