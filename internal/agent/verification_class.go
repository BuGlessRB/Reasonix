package agent

import (
	"context"
	"strings"
	"sync"

	"reasonix/internal/evidence"
)

// The verifier table has the read-only table's shortcoming: a project's check is
// often its own script, and one the table cannot name is not a check to the
// ledger — so a turn that did verify is told to verify. Same escalation
// shellsafe already uses, pointed at the other table.
const verificationClassSystemPrompt = `You classify one shell command as a correctness check or not.

CHECK: running it decides whether code is correct or well-formed and reports the
verdict through its exit status — a test run, a type check, a linter or static
analyser, a compile whose only purpose is to see whether it compiles, or a
project script that runs those.

NOT: it changes code or files; it formats or fixes in place; it only lists,
collects or plans work without running it; it prints information; it installs,
builds an artifact, or manages the environment.

Judge the command in front of you, not the program in general. A runner told to
select nothing checks nothing: -run with a pattern that matches no test,
--collect-only, --dry-run, --list, -h. A command that writes what it inspects
(--fix, --write, -w, -i, --update, a > redirection) is NOT a check.

Answer with exactly one word: CHECK or NOT.
If you cannot tell what it would do, answer NOT.`

// verificationClassCache remembers one verdict per command for the process, so
// the audited ledger classifies the same command the same way every time.
type verificationClassCache struct {
	mu sync.RWMutex
	by map[string]bool
}

func (c *verificationClassCache) get(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.by[key]
	return v, ok
}

func (c *verificationClassCache) put(key string, isCheck bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.by == nil {
		c.by = map[string]bool{}
	}
	c.by[key] = isCheck
}

var sharedVerificationClass verificationClassCache

// commandIsCheckByEscalation reports whether a command the static table did not
// recognise runs a check, answering false for anything it cannot establish: no
// provider, a timeout, a reply that is not one of the two words. It widens only
// what counts as a check — whether that check passed stays the host's reading of
// the exit status, so a classifier can never certify anything.
func (a *Agent) commandIsCheckByEscalation(ctx context.Context, command string) bool {
	if a == nil || a.triageProvider() == nil {
		return false
	}
	command = strings.TrimSpace(command)
	if command == "" || len(command) > commandClassMaxCommand {
		return false
	}
	// Only when the answer changes something. Without a write the ledger cannot
	// yet call verified, a check costs nothing to miss — and paying for a
	// classification on every ls would be the expensive way to learn that.
	if !a.verificationOwed() {
		return false
	}
	if verdict, ok := sharedVerificationClass.get(command); ok {
		return verdict
	}
	verdict := a.askClass(ctx, verificationClassSystemPrompt, command, "CHECK")
	sharedVerificationClass.put(command, verdict)
	return verdict
}

// verificationOwed reports whether this turn has a write the ledger cannot yet
// call verified — the only state in which naming one more command a check
// changes the outcome.
func (a *Agent) verificationOwed() bool {
	if a == nil || a.task.ledger == nil {
		return false
	}
	writer, hasWriter := a.mutationBaseline(a.deliveryProfile)
	if !hasWriter {
		return false
	}
	verified, _ := a.postWriteVerification(writer)
	return !verified
}

// runsVerification is the static table with the escalation behind it. The table
// stays the fast path and answers everything it knows.
func (a *Agent) runsVerification(ctx context.Context, command string) bool {
	if evidence.CommandRunsVerification(command) {
		return true
	}
	return a.commandIsCheckByEscalation(ctx, command)
}
