package control

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/evidence"
	"reasonix/internal/i18n"
)

// A turn that ends owing verification has not failed: the host read what is
// missing off its own receipts, so asking the user to press continue asks them
// to relay a message the host wrote. What bounds the work instead is the gap —
// admitted only while it is still shrinking.
const readinessStallRounds = 2

// continueUntilReady runs the missing requirements as further turns. turnErr is
// the outcome of the turn just finished; the returned error is the outcome of
// the last one run, so a caller cannot tell whether one turn or four produced it.
func (o *turnOrchestrator) continueUntilReady(ctx context.Context, turnErr error) error {
	best, stall := -1, 0
	for {
		var readinessErr *agent.FinalReadinessError
		if !errors.As(turnErr, &readinessErr) {
			// Ready, or a failure that has nothing to do with readiness.
			return turnErr
		}
		gap := len(readinessErr.Missing)
		switch {
		case best < 0 || gap < best:
			best, stall = gap, 0
		default:
			// Compared against the best round so far, not the last one, so a gap
			// that oscillates cannot reset the counter and run forever.
			stall++
			if stall >= readinessStallRounds {
				return turnErr
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		prompt := readinessContinuationPrompt(o.c.goalTodos(), readinessErr.Reason)
		if prompt == "" {
			return turnErr
		}
		// The continuation inherits the finished turn's receipts. Without that
		// the next run starts from an empty ledger and the gap reads as closed
		// because the record of it was dropped, not because it was filled.
		if o.c.executor == nil || !o.c.executor.PrepareReadinessContinuation() {
			return turnErr
		}
		o.c.noticeDetail(i18n.M.ReadinessContinuing, prompt)
		turnErr = o.runOrchestratedTurn(ctx, orchestratedTurn{input: prompt, raw: prompt, synthetic: true})
	}
}

// readinessContinuationPrompt states what the turn still owes. It is the host's
// own account, so it names the requirement rather than asking the model whether
// it agrees one is outstanding.
func readinessContinuationPrompt(todos []evidence.TodoItem, reason string) string {
	var parts []string
	if incomplete := evidence.IncompleteTodos(todos); len(incomplete) > 0 {
		var b strings.Builder
		b.WriteString("these tasks are still incomplete:")
		for _, t := range incomplete {
			fmt.Fprintf(&b, "\n  - %s (%s)", t.Content, t.Status)
		}
		parts = append(parts, b.String())
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		parts = append(parts, reason)
	}
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("This turn ended with work still outstanding:\n")
	for _, p := range parts {
		b.WriteString("- " + p + "\n")
	}
	b.WriteString("Finish it now. Do the remaining work, then verify it and record the outcome.")
	return b.String()
}
