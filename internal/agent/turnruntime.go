package agent

import (
	"reasonix/internal/completion"
	"reasonix/internal/taskpolicy"
)

// turnRuntime is the host state for exactly one Agent.Run. beginRunTurn builds
// it in a single assignment, so a field added here starts the next turn zeroed.
// State an external caller arms before a Run lives in pendingTurn; state that
// outlives the Run lives in taskRuntime or sessionRuntime.
type turnRuntime struct {
	runMaxSteps       int
	runMaxStepsKey    string
	runLimitHostOwned bool

	emptyFinalBlocks   int
	handoffNudges      int
	usedAnyTool        bool
	contextToolRepairs int
	graceRound         bool
	recoveryGraceRound bool

	executorHandoff bool
	input           string
	workDurationMs  func() int64

	// budget is the turn's spend axis: tokens, money, wall clock.
	budget runBudget
	// landCause records why the grace round was armed, so the pause the Run
	// ends with names the axis that actually stopped it.
	landCause landCause

	// turnInput is this run's task text. The contract is rebuilt from it and
	// the ledger whenever a live view is needed, so one replay serves both the
	// per-round observation and the end-of-turn record.
	turnInput string
	// completion is the report built as the turn ends; the host reads it while
	// emitting TurnDone, before the next turn resets this state.
	completion *completion.Report
	// deliveryCriteriaEstablished may inherit an unfinished canonical task list
	// on continuation, but the flag itself is recomputed every turn.
	deliveryCriteriaEstablished bool
	deliveryScopeActive         bool
	// readinessRecovered marks a run that started with evidence preserved from
	// (or a pending recovery of) a prior readiness failure, so the final
	// allowed audit can report Recovered=true.
	readinessRecovered bool

	// recoveryTaskSummary is the bounded task text for this Agent.Run. It lets
	// a shared recovery gate review sub-agent mutations against the child
	// task, rather than the root controller transcript.
	recoveryTaskSummary string

	// policy is frozen at the start of the Run and never observes a mid-turn
	// SetAgentPreset change. policySet marks that beginRunTurn derived it.
	policy    taskpolicy.TaskPolicy
	policySet bool

	// reviewWarnings are warn-level findings to surface in the final summary.
	reviewWarnings []string
}

// pendingTurn is what someone outside the Run arms for the next one: a
// sub-agent spawner, the turn that just failed readiness, or the fork capture.
// It is deliberately not in turnRuntime — beginRunTurn builds that fresh, and
// state armed before it exists would be wiped by the same assignment that makes
// turnRuntime safe.
type pendingTurn struct {
	// preserveEvidence makes the next Run keep the turn evidence ledger instead
	// of resetting it, so a review_report completion nudge can cite the read
	// receipts the subagent already earned. Consumed by that Run.
	preserveEvidence bool
	// deliveryRecovery is armed only when this agent exhausts final readiness.
	// An explicit host recovery action can consume it to preserve the failed
	// turn's receipts once; an ordinary user turn still resets evidence.
	deliveryRecovery bool
}
