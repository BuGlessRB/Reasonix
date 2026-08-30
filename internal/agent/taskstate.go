package agent

import "reasonix/internal/evidence"

// taskRuntime is the host state shared by every Agent.Run continuing one
// delivery scope: one ledger, one bill, one set of failure budgets. Its
// lifetime sits between the session and the turn — SetSession replaces it
// wholesale, beginRunTurn restarts it when the scope changes, and state valid
// for exactly one Run lives in perTurnState instead.
type taskRuntime struct {
	scopeID    string
	checkpoint evidence.DeliveryCheckpoint
	// baselineCriteria names what each rewritten file's criteria said first,
	// keyed by path. The first capture of a path wins.
	baselineCriteria map[string]evidence.TestCriterion
	ledger           *evidence.Ledger
	outcome          *evidence.OutcomeTracker
	budget           runBudget
	// witness holds, per changed path, the lines a later output has to carry to
	// have shown that change. It is working state, not evidence: the verdict
	// lands on the receipt, so the ledger never holds file content.
	witness map[string][]string
	// todoRevs counts what the task list did, on three axes because they answer
	// different questions and one number for all three reads a rewrite as work.
	todoRevs todoRevisions
}

// todoRevisions separates writing the plan from changing it and from executing
// it. progress counts only host-observed execution movement, so a turn cannot
// renew it by restating its own list.
type todoRevisions struct {
	content  int
	plan     int
	progress int
}

// restartLedger begins a new task's accounting. It is written as one assignment
// so a field added to taskRuntime resets by default; the fields carried forward
// are named because each answers to its own condition in beginRunTurn.
func (t *taskRuntime) restartLedger() {
	*t = taskRuntime{
		scopeID:    t.scopeID,
		checkpoint: t.checkpoint,
		ledger:     t.ledger,
		outcome:    evidence.NewOutcomeTracker(),
		budget:     runBudget{limit: t.budget.limit},
	}
	t.ledger.Reset()
}
