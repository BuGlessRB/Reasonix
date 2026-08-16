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
