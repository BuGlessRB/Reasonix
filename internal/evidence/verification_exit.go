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
