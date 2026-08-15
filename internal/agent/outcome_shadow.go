package agent

import (
	"reasonix/internal/event"
	"reasonix/internal/evidence"
)

// observeOutcomeShadow scores one round's receipts by outcome — information
// gathered, verifications run, state transitions, unverified change carried —
// and offers the sample to any sink collecting them. It is an instrument, not
// a guard: nothing reads the numbers back to decide what the turn may do.
func (a *Agent) observeOutcomeShadow(cancelled bool, receiptMark int) {
	if cancelled || a.task.ledger == nil {
		return
	}
	if a.task.outcome == nil {
		a.task.outcome = evidence.NewOutcomeTracker()
	}
	event.RecordOutcomeProgress(a.svc.sink, a.task.outcome.ScoreRound(a.task.ledger.ReceiptsSince(receiptMark)))
}

// ledgerMark is the receipt count to score a round against, or zero before any
// ledger exists.
func (a *Agent) ledgerMark() int {
	if a.task.ledger == nil {
		return 0
	}
	return a.task.ledger.Len()
}
