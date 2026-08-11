package agent

import (
	"context"
	"fmt"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

// progressGuard watches one turn's investigation runway. It replaced a ladder
// of fixed round counts: those fired at a particular round however well the
// turn had been going, and a single incidental new read reset them, so they
// both cut off honest investigation and missed real loops. The account has
// neither edge — it drains at the rate the turn is failing to produce.
type progressGuard struct {
	runway evidence.Runway
	spoken bool // the spent transition is stated once per drain
}

type goalStuckSignal struct {
	limit  int
	key    string
	reason string
}

func (g *progressGuard) reset() {
	g.runway.Reset()
	g.spoken = false
}

// Receipt aliases the evidence receipt for the guard's signature.
type Receipt = evidence.Receipt

// applyBatchGuards collects this round's signals — storm breaker (failure
// fixation), the investigation runway, evidence nudge — and lets the arbiter
// deliver them as one tail. One scoring pass serves them all: the runway, the
// EBM trigger, the reasoning governor and the trajectory record read the same
// sample.
func (a *Agent) applyBatchGuards(ctx context.Context, cancelled bool, calls []provider.ToolCall, outcomes []toolOutcome, results []string, receiptMark int) goalStuckSignal {
	if cancelled {
		return goalStuckSignal{}
	}
	storm := a.applyStormBreaker(calls, outcomes, receiptMark)
	receipts := a.evidence.ReceiptsSince(receiptMark)
	sample := a.scoreRound(receipts)
	progress := a.applyProgressGuard(&sample, receipts, outcomes, receiptMark)
	shadow := a.observeRound(&sample, outcomes)
	a.applyInterventions(results, outcomes, storm, progress, shadow)
	a.observeDelegationAdmission(calls)
	if _, scoped := DeliveryExecutionScopeFromContext(ctx); !scoped {
		return goalStuckSignal{}
	}
	if storm.stuckReason != "" {
		return goalStuckSignal{limit: stormBreakThreshold, key: "goal repeated host outcome", reason: storm.stuckReason}
	}
	if progress.stuckReason != "" {
		return goalStuckSignal{limit: evidence.RunwayStart, key: "goal investigation runway spent", reason: progress.stuckReason}
	}
	return goalStuckSignal{}
}

// scoreRound folds the batch's receipts into the turn's single round scorer.
func (a *Agent) scoreRound(receipts []Receipt) evidence.OutcomeSample {
	if a.evidence == nil {
		return evidence.OutcomeSample{}
	}
	if a.outcome == nil {
		a.outcome = evidence.NewOutcomeTracker()
	}
	return a.outcome.ScoreRound(receipts)
}

// resetTurnEvidence clears the ledger and the round scorer together. The task
// budget resets with them: a fresh ledger is what "a new task" means here, and
// a continuation keeps both.
func (a *Agent) resetTurnEvidence() {
	a.evidence.Reset()
	a.progress.reset()
	a.stormSig, a.stormCount, a.blockedTurnStreak = "", 0, 0
	a.outcome = evidence.NewOutcomeTracker()
	a.ebm = ebmState{}
	a.governor = governorState{}
	a.taskBudget = runBudget{limit: a.taskBudget.limit}
}

// observeRound folds the round's sample into the policies that only watch it:
// the EBM nudge (which can act), the reasoning governor, and the recorders.
func (a *Agent) observeRound(sample *evidence.OutcomeSample, outcomes []toolOutcome) intervention {
	if a.evidence == nil {
		return intervention{}
	}
	iv := a.applyEBM(sample, outcomes)
	a.applyGovernor(sample)
	a.armGovernorCapture(*sample)
	event.RecordOutcomeProgress(a.sink, *sample)
	a.observeContractRound()
	return iv
}

// applyProgressGuard settles the round against the turn's runway and states what
// the host sees. It never instructs: while the balance drains it reports the
// balance, and on the round that empties it, it reports that readiness has stood
// down — the model decides what to do with either.
func (a *Agent) applyProgressGuard(sample *evidence.OutcomeSample, receipts []Receipt, outcomes []toolOutcome, receiptMark int) intervention {
	if a.evidence == nil || len(outcomes) == 0 {
		return intervention{}
	}
	// Rounds where nothing succeeded are the storm breaker's jurisdiction
	// (same-failure fixation); the runway owns the storm-blind case — rounds
	// that keep SUCCEEDING without getting anywhere.
	anySuccess := false
	for _, r := range receipts {
		if r.Success {
			anySuccess = true
			break
		}
	}
	if !anySuccess {
		return intervention{}
	}
	state := a.progress.runway.Settle(*sample)
	sample.Runway, sample.RunwayDry, sample.RunwayIdle, sample.RunwaySpent =
		state.Balance, state.Dry, state.Idle, state.Spent
	if !state.Low && !state.Spent {
		// Earning the account back out of the low band closes the episode: a
		// later drain is a new one and gets its own statement and stand-down.
		a.progress.spoken = false
		return intervention{}
	}
	// Within one drain the host says its piece once. Narrating every further
	// round would be the nagging this account replaced.
	if a.progress.spoken {
		return intervention{}
	}
	level, tier := event.LevelInfo, verdictAdvise
	if state.Spent {
		level, tier = event.LevelWarn, verdictLand
	}
	iv := intervention{
		verdict:  tier,
		guidance: runwayObservation(state),
		notice:   noticeFor(event.NoticeCodeProgressGuard, level, runwayNoticeText(state), runwayDetail(state)),
	}
	if state.Spent {
		a.progress.spoken = true
		a.armLoopGuardPass(receiptMark)
		// Shown to the user as the pause's explanation, so it reads as a
		// sentence; runwayDetail carries the numbers on the notice instead.
		iv.stuckReason = fmt.Sprintf(
			"the turn's investigation budget ran out: %d rounds in a row ran, changed, or verified nothing", state.Idle)
	}
	return iv
}

// runwayObservation is what the model reads: the host's own measurements, and
// at the end the host's own change of behavior. No imperative — a turn told
// what is true decides better than one told what to do.
func runwayObservation(state evidence.RunwayState) string {
	if state.Spent {
		return fmt.Sprintf(
			"[host] This turn's investigation runway is spent: %d rounds in a row ran, changed, or verified nothing, %d of them producing nothing new at all. The host has stopped requiring further receipts for this turn — an answer stating what was established and what remains unverified is now accepted.",
			state.Idle, state.Dry)
	}
	if state.Dry > 0 {
		return fmt.Sprintf(
			"[host] %d rounds in a row produced nothing new — nothing read, run, or changed that the host had not already seen. At this rate the turn's investigation budget covers about %d more rounds.",
			state.Dry, state.Rounds)
	}
	return fmt.Sprintf(
		"[host] %d rounds in a row ran, changed, or verified nothing. At this rate the turn's investigation budget covers about %d more rounds.",
		state.Idle, state.Rounds)
}

func runwayNoticeText(state evidence.RunwayState) string {
	if state.Spent {
		return "The assistant's investigation budget is spent; the host stopped asking it for more evidence."
	}
	return "The assistant is running low on investigation budget without landing new evidence."
}

func runwayDetail(state evidence.RunwayState) string {
	return fmt.Sprintf("runway %d (~%d rounds), %d dry rounds, %d rounds without acting",
		state.Balance, state.Rounds, state.Dry, state.Idle)
}

// armLoopGuardPass records that a loop guard fired this user turn.
// receiptMark is the evidence-ledger receipt count from just before the
// guarded batch ran, so a successful write or command receipt recorded after
// it counts as real progress and revokes the pass (see loopGuardAllowsFinal).
func (a *Agent) armLoopGuardPass(receiptMark int) {
	a.loopGuardArmed = true
	a.loopGuardReceiptMark = receiptMark
}

// loopGuardAllowsFinal reports whether final readiness should stand down: a
// guard fired this user turn and no successful write or command receipt has
// landed since. The missing receipts are exactly what the blocker prevents —
// demanding them would restart the loop the guard broke — while bookkeeping
// (ask, todo_write, complete_step) keeps the pass and real progress revokes it.
func (a *Agent) loopGuardAllowsFinal() bool {
	if a == nil || !a.loopGuardArmed {
		return false
	}
	if a.evidence == nil {
		return true
	}
	return !a.evidence.HasWriteOrCommandSince(a.loopGuardReceiptMark)
}
