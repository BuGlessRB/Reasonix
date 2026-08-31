package eventwire

import "reasonix/internal/event"

// CompletionValidation is the JSON form of event.CompletionValidationInfo.
type CompletionValidation struct {
	Mode       string `json:"mode"`
	Outcome    string `json:"outcome"`
	Attempt    int    `json:"attempt"`
	DurationMs int64  `json:"durationMs,omitempty"`
	ErrorClass string `json:"errorClass,omitempty"`
}

func toWireCompletionSummary(c *event.CompletionSummaryInfo) *CompletionSummary {
	if c == nil {
		return nil
	}
	return &CompletionSummary{
		Preset:             c.Preset,
		Verdict:            c.Verdict,
		Mutations:          c.Mutations,
		ChecksPassed:       c.ChecksPassed,
		ChecksFailed:       c.ChecksFailed,
		ChecksSuppressed:   c.ChecksSuppressed,
		Review:             c.Review,
		GapKinds:           append([]string(nil), c.GapKinds...),
		ConstraintDegraded: c.ConstraintDegraded,
		Floor:              c.Floor,
		Attention:          c.Attention,
	}
}

func toWireCompletionValidation(v *event.CompletionValidationInfo) *CompletionValidation {
	if v == nil {
		return nil
	}
	return &CompletionValidation{
		Mode:       v.Mode,
		Outcome:    v.Outcome,
		Attempt:    v.Attempt,
		DurationMs: v.DurationMs,
		ErrorClass: v.ErrorClass,
	}
}
