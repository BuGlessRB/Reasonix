package agent

import (
	"testing"

	"reasonix/internal/evidence"
)

// The escalation may only widen what counts as a check by one narrow answer, so
// anything that is not a bare CHECK has to leave the static verdict standing.
func TestVerificationClassOnlyAcceptsABareCheck(t *testing.T) {
	for _, reply := range []string{"CHECK", "check", "  Check \n"} {
		if !parseClassVerdict(reply, "CHECK") {
			t.Errorf("parseClassVerdict(%q) = false, want true", reply)
		}
	}
	refused := []string{
		"NOT",
		"",
		"CHECK, if the suite is configured",
		"probably CHECK",
		"CHECK NOT",
		"it runs the tests",
		"CHE", // truncated at the token cap
	}
	for _, reply := range refused {
		if parseClassVerdict(reply, "CHECK") {
			t.Errorf("parseClassVerdict(%q) = true, want false: only a bare verdict counts", reply)
		}
	}
}

func TestVerificationClassCacheAnswersOnce(t *testing.T) {
	var c verificationClassCache
	if _, ok := c.get("./scripts/verify.sh"); ok {
		t.Fatal("empty cache reported a verdict")
	}
	c.put("./scripts/verify.sh", true)
	if got, ok := c.get("./scripts/verify.sh"); !ok || !got {
		t.Fatalf("cache returned (%v, %v), want the stored verdict", got, ok)
	}
	c.put("./scripts/build.sh", false)
	if got, ok := c.get("./scripts/build.sh"); !ok || got {
		t.Fatalf("cache returned (%v, %v) for a non-check", got, ok)
	}
}

// Without a provider there is nothing to escalate to, and the static table's
// answer has to stand rather than the turn gaining a check it never ran.
func TestCommandIsCheckWithoutProviderRefuses(t *testing.T) {
	var a Agent
	if a.commandIsCheckByEscalation(t.Context(), "./scripts/verify.sh") {
		t.Fatal("classified a check with no provider to ask")
	}
	if (&Agent{}).commandIsCheckByEscalation(t.Context(), "") {
		t.Fatal("classified an empty command as a check")
	}
}

// The table stays the fast path: a command it already knows must not reach the
// escalation at all, which is also what keeps the common turn free.
func TestRunsVerificationAnswersFromTheTableWithoutEscalating(t *testing.T) {
	var a Agent // no provider: an escalation here could only answer false
	for _, command := range []string{"go test ./...", "go vet ./...", "npm run lint"} {
		if !evidence.CommandRunsVerification(command) {
			t.Fatalf("precondition: the table should already know %q", command)
		}
		if !a.runsVerification(t.Context(), command) {
			t.Fatalf("runsVerification(%q) = false; the table's answer was lost", command)
		}
	}
}

// A command the table does not know stays unrecognised when nothing is owed.
// Escalating there would buy nothing and be paid for on every ls.
func TestCommandIsCheckSkippedWhenNoVerificationIsOwed(t *testing.T) {
	a := &Agent{}
	if a.verificationOwed() {
		t.Fatal("a turn with no ledger owes a verification")
	}
	if a.commandIsCheckByEscalation(t.Context(), "./scripts/verify.sh") {
		t.Fatal("escalated for a turn that owes no verification")
	}
}

// The point of the whole gate: a classifier may say a command is a check, and
// it still cannot say the check passed. That verdict is read off the exit
// status, so a check that ran and failed stays failed however it was
// recognised.
func TestEscalationCannotTurnAFailedCheckIntoAPass(t *testing.T) {
	failed := evidence.Receipt{
		ToolName:     "bash",
		Command:      "./scripts/verify.sh",
		Success:      false,
		Verification: evidence.VerificationFailed,
	}
	if evidence.VerificationOutcome(failed) != evidence.VerificationFailed {
		t.Fatal("a failed check did not read as failed")
	}
	passed := failed
	passed.Success = true
	passed.Verification = evidence.VerificationPassed
	if evidence.VerificationOutcome(passed) != evidence.VerificationPassed {
		t.Fatal("a passing check did not read as passed")
	}
}
