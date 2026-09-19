package sandbox

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunnerReportRequiresCompleteTypedRecord(t *testing.T) {
	cause := errors.New("exit status 126")
	for _, phase := range []string{WindowsSandboxFailureAuthorization, WindowsSandboxFailureDependency, WindowsSandboxFailureLaunch} {
		var report bytes.Buffer
		if err := writeRunnerReport(&report, phase, errors.New("failed "+phase)); err != nil {
			t.Fatal(err)
		}
		result := readRunnerReport(&report, cause)
		got, _, ok := RunnerFailureFromError(result)
		if !ok || got != phase || !errors.Is(result, cause) {
			t.Fatalf("phase=%s err=%v", got, result)
		}
	}
	for _, data := range []string{"", `{"version":1`, `{"version":2,"phase":"launch"}`, `{"version":1,"phase":"execution"}`, strings.Repeat("x", runnerReportLimit+1)} {
		err := readRunnerReport(strings.NewReader(data), cause)
		if _, _, ok := RunnerFailureFromError(err); ok {
			t.Fatalf("untrusted/incomplete record accepted: %q", data)
		}
		if !errors.Is(err, cause) {
			t.Fatalf("original exit lost: %v", err)
		}
	}
}
