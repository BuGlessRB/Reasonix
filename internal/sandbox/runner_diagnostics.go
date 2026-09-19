package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RunnerFailure is created only from the host-owned helper report channel,
// never from a command's stdout/stderr or its numeric exit status.
type RunnerFailure struct {
	Phase  string
	Detail string
	Cause  error
}

func (e *RunnerFailure) Error() string { return e.Detail }
func (e *RunnerFailure) Unwrap() error { return e.Cause }

func RunnerFailureFromError(err error) (string, string, bool) {
	var failure *RunnerFailure
	if errors.As(err, &failure) {
		return failure.Phase, failure.Detail, true
	}
	return "", "", false
}

const runnerReportLimit = 4096
const runnerReportEnvironment = "REASONIX_INTERNAL_RUNNER_REPORT_HANDLE"

type runnerReport struct {
	Version int    `json:"version"`
	Phase   string `json:"phase"`
	Detail  string `json:"detail"`
}

func writeRunnerReport(w io.Writer, phase string, err error) error {
	if w == nil {
		return nil
	}
	detail := err.Error()
	if len(detail) > 512 {
		detail = detail[:512]
	}
	return json.NewEncoder(w).Encode(runnerReport{Version: 1, Phase: phase, Detail: detail})
}

func readRunnerReport(r io.Reader, cause error) error {
	if cause == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(r, runnerReportLimit+1))
	if err != nil || len(data) == 0 {
		return cause
	}
	var report runnerReport
	if len(data) > runnerReportLimit || json.Unmarshal(data, &report) != nil || report.Version != 1 {
		// Invalid/incomplete reports cannot prove the command never started.
		return fmt.Errorf("invalid sandbox runner report: %w", cause)
	}
	switch report.Phase {
	case WindowsSandboxFailureAuthorization, WindowsSandboxFailureDependency, WindowsSandboxFailureLaunch:
		return &RunnerFailure{Phase: report.Phase, Detail: report.Detail, Cause: cause}
	default:
		return fmt.Errorf("unknown sandbox runner phase: %w", cause)
	}
}
