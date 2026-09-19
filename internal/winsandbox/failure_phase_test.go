package winsandbox

import (
	"errors"
	"testing"
)

func TestFailurePhaseOfUsesTypedRunnerStage(t *testing.T) {
	base := errors.New("same text for every stage")
	for _, want := range []FailurePhase{FailureAuthorization, FailureDependency, FailureLaunch} {
		got, ok := FailurePhaseOf(failureAt(want, base))
		if !ok || got != want {
			t.Fatalf("phase = %q, %v; want %q", got, ok, want)
		}
	}
	if _, ok := FailurePhaseOf(base); ok {
		t.Fatal("an untyped post-start error must remain in the execution lane")
	}
}
