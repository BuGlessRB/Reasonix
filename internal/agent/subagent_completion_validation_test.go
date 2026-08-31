package agent

import (
	"testing"

	"reasonix/internal/event"
)

func TestSubagentProgressForwardsCompletionValidation(t *testing.T) {
	parent := &recordSink{}
	tracker := &subagentProgressTracker{sink: parent}
	tracker.wrap().Emit(event.Event{Kind: event.CompletionValidation, CompletionValidation: &event.CompletionValidationInfo{
		Mode: CompletionValidationEnforce, Outcome: "complete", Attempt: 1,
	}})

	validations := parent.kinds(event.CompletionValidation)
	if len(validations) != 1 || validations[0].CompletionValidation == nil || validations[0].CompletionValidation.Outcome != "complete" {
		t.Fatalf("completion validation audit = %+v, want one forwarded event", validations)
	}
}
