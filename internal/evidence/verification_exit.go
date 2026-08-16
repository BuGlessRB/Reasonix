package evidence

import (
	"strings"

	"reasonix/internal/shellparse"
	"reasonix/internal/shellsafe"
)

// VerificationExitConclusive reports whether a zero exit status would prove the
// verification inside command passed. A pipe, `;`, or `||` can hand the status
// to a later stage — `go test ./... | head` exits 0 on a failing suite — while
// `&&` still short-circuits on the suite. A command no static pass can
// decompose is inconclusive by the same caution.
func VerificationExitConclusive(command string) bool {
	proven, ok := shellparse.ExitZeroImplies(command)
	if !ok {
		return false
	}
	for _, segment := range proven {
		normalized, safeRedirects := shellsafe.NormalizeBashSafeRedirectsForMatch(segment)
		if !safeRedirects {
			continue
		}
		if fields, malformed := shellparse.StaticFields(normalized); malformed == "" && bashSegmentIsVerification(fields) {
			return true
		}
	}
	return false
}

// VerificationOutcomeFromPipeStatus reads the verdict off per-stage statuses
// the host captured, so a check whose exit status a later stage swallowed is
// still answered rather than shrugged at. It returns "" when the statuses do
// not line up with the pipeline, or when no stage of it is a verification.
func VerificationOutcomeFromPipeStatus(command string, status []int) string {
	if len(status) == 0 {
		return ""
	}
	stages, ok := shellparse.SinglePipelineStages(command)
	if !ok || len(stages) != len(status) {
		return ""
	}
	verdict := ""
	for i, stage := range stages {
		if !bashContainsVerificationSegment(stage) {
			continue
		}
		if status[i] != 0 {
			return VerificationFailed
		}
		verdict = VerificationPassed
	}
	return verdict
}

// HasVerificationCommandAfter reports whether a check ran after the boundary at
// all, whatever it concluded. A turn the model declared blocked still owes the
// run that establishes the blocker; what it cannot owe is a passing one.
func (l *Ledger) HasVerificationCommandAfter(after int) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts[max(after+1, 0):] {
		if r.ToolName == "bash" && bashContainsVerificationSegment(r.Command) {
			return true
		}
	}
	return false
}

// HasFailedVerificationAfter reports whether any check still stands failed
// after the boundary. Without it `go test; go vet` reads as verified on a red
// suite, since the shell's status is go vet's. Outcomes fold per check, so
// re-running one until it passes clears it.
func (l *Ledger) HasFailedVerificationAfter(after int) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	latest := map[string]string{}
	for _, r := range l.receipts[max(after+1, 0):] {
		if r.ToolName != "bash" || !CommandRunsVerification(r.Command) {
			continue
		}
		if outcome := VerificationOutcome(r); outcome != "" {
			latest[VerificationIdentity(r.Command)] = outcome
		}
	}
	for _, outcome := range latest {
		if outcome == VerificationFailed {
			return true
		}
	}
	return false
}

// HasBlockedConclusionAfter reports whether the model declared, after the
// boundary, that the task cannot be completed as specified. The tool checked
// the claim's evidence before the receipt could succeed.
func (l *Ledger) HasBlockedConclusionAfter(after int) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts[max(after+1, 0):] {
		if r.ToolName == "conclude_blocked" && r.Success {
			return true
		}
	}
	return false
}

// LatestUnreadableVerificationAfter returns the most recent check that ran
// after the boundary and whose outcome the host could not read. Telling a model
// to "run a verification" when it just ran one it cannot see the result of
// sends it round the same loop; naming the command is what breaks it.
func (l *Ledger) LatestUnreadableVerificationAfter(after int) (Receipt, bool) {
	if l == nil {
		return Receipt{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.receipts) - 1; i > after && i >= 0; i-- {
		if r := l.receipts[i]; r.Success && r.Verification == VerificationInconclusive {
			return r, true
		}
	}
	return Receipt{}, false
}

// CommandRunsVerification reports whether command runs a verification at all,
// whatever else it also runs. IsDeliveryVerificationCommand asks the stricter
// question — whether the command runs *only* verification and read-only work —
// and stays the gate for delivery sign-off. This one decides whether a receipt
// carries a verification verdict, so a check is not discarded for its company.
func CommandRunsVerification(command string) bool {
	return bashContainsVerificationSegment(command)
}

// VerificationOutcome is a verification receipt's host-read verdict:
// VerificationPassed, VerificationFailed, or "" when the shell's exit status
// proved neither. A receipt the host never classified carries only its own
// outcome, so that is what it answers with.
func VerificationOutcome(r Receipt) string {
	switch r.Verification {
	case VerificationPassed:
		return VerificationPassed
	case VerificationFailed:
		return VerificationFailed
	case VerificationInconclusive, VerificationNotRun:
		return ""
	}
	if r.Success {
		return VerificationPassed
	}
	return VerificationFailed
}

func verificationPassed(r Receipt) bool {
	return VerificationOutcome(r) == VerificationPassed
}

// VerificationIdentity reduces command to the verification it runs, so the same
// check re-run inside a different wrapper stays one item whose latest outcome
// supersedes the earlier one, instead of becoming a second item that the fresh
// run never clears. Commands whose segments cannot be isolated keep their own
// text as identity.
func VerificationIdentity(command string) string {
	command = strings.TrimSpace(command)
	segments, _, ok := shellparse.SplitTopLevel(command)
	if !ok {
		return command
	}
	var verifiers []string
	for _, segment := range segments {
		normalized, safeRedirects := shellsafe.NormalizeBashSafeRedirectsForMatch(segment)
		if !safeRedirects {
			continue
		}
		fields, malformed := shellparse.StaticFields(normalized)
		if malformed != "" || !bashSegmentIsVerification(fields) {
			continue
		}
		verifiers = append(verifiers, strings.Join(fields, " "))
	}
	if len(verifiers) == 0 {
		return command
	}
	return strings.Join(verifiers, " && ")
}
