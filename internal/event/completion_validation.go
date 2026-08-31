package event

const UsageSourceCompletionEvaluator = "completion-evaluator"

// TurnOutcomeCompletionUncertain marks a resumable stop after the completion
// validator could not confirm the turn's result. The result and completed work
// are kept; clients show an informational status, never a send failure.
const TurnOutcomeCompletionUncertain = "completion_uncertain"

// CompletionValidationInfo is the content-free audit of one completion
// validation: the configured mode, the validator's outcome, the attempt
// number within the run, the call duration, and an error class. It never
// carries the candidate text or the evaluator's reason.
type CompletionValidationInfo struct {
	Mode       string // off | shadow | enforce
	Outcome    string // complete | continue | needs_user | blocked | uncertain | error
	Attempt    int    // 1-based evaluation attempt within the run
	DurationMs int64
	ErrorClass string // timeout | invalid_output | unavailable | over_budget | ""; empty when Outcome is a verdict
}
