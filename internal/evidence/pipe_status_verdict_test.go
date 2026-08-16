package evidence

import "testing"

func TestVerificationOutcomeFromPipeStatus(t *testing.T) {
	const masked = "go test ./... 2>&1 | tail -5"

	if got := VerificationOutcomeFromPipeStatus(masked, []int{0, 0}); got != VerificationPassed {
		t.Fatalf("passing suite = %q, want %q", got, VerificationPassed)
	}
	// The shape the whole probe exists for: the suite failed, `tail` succeeded,
	// and the shell reported zero.
	if got := VerificationOutcomeFromPipeStatus(masked, []int{1, 0}); got != VerificationFailed {
		t.Fatalf("failing suite behind a pipe = %q, want %q", got, VerificationFailed)
	}
	if got := VerificationExitConclusive(masked); got {
		t.Fatal("the exit status alone must stay inconclusive for this shape")
	}
}

func TestVerificationOutcomeFromPipeStatusStaysSilentWhenUnsure(t *testing.T) {
	cases := []struct {
		name    string
		command string
		status  []int
	}{
		{"no report", "go test ./... | tail -5", nil},
		{"status count mismatch", "go test ./... | tail -5", []int{0}},
		{"not a single pipeline", "go vet ./... && go test ./...", []int{0, 0}},
		{"no verification stage", "cat log | tail -5", []int{0, 0}},
	}
	for _, tc := range cases {
		if got := VerificationOutcomeFromPipeStatus(tc.command, tc.status); got != "" {
			t.Errorf("%s: verdict = %q, want none", tc.name, got)
		}
	}
}

// The shape that let a red suite read as verified: two checks in one turn, one
// failing, and the shell's exit status belonging to the one that passed.
func TestFailedVerificationIsNotAnsweredByAnotherPassingOne(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "edit_file", Success: true, Write: true, Mutation: true, MutationEvidence: MutationProven, Paths: []string{"quota.go"}})
	l.Record(Receipt{ToolName: "bash", Success: false, Command: "go test ./...", Verification: VerificationFailed})
	l.Record(Receipt{ToolName: "bash", Success: true, Command: "go vet ./...", Verification: VerificationPassed})

	writer, ok := l.LatestProvenMutationIndex()
	if !ok {
		t.Fatal("expected a proven write")
	}
	if !l.HasSuccessfulVerificationCommandAfter(writer) {
		t.Fatal("go vet did pass — the older gate saw only this")
	}
	if !l.HasFailedVerificationAfter(writer) {
		t.Fatal("the failing suite must still count against the change")
	}
}

// Re-running the same check until it passes is ordinary work, not a standing
// failure: outcomes fold per check and only the latest one counts.
func TestReRunClearsAnEarlierFailure(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "edit_file", Success: true, Write: true, Mutation: true, MutationEvidence: MutationProven, Paths: []string{"quota.go"}})
	l.Record(Receipt{ToolName: "bash", Success: false, Command: "go test ./...", Verification: VerificationFailed})
	l.Record(Receipt{ToolName: "bash", Success: true, Command: "go test ./...", Verification: VerificationPassed})

	writer, _ := l.LatestProvenMutationIndex()
	if l.HasFailedVerificationAfter(writer) {
		t.Fatal("the re-run passed, so nothing stands failed")
	}
}
