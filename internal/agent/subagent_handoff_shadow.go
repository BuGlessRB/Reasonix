// subagent_handoff_shadow.go — how often a child closes the way it was asked to.
package agent

import (
	"context"
	"errors"
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// Nothing here decides anything: no prompt byte moves, no notice is emitted, no
// parent handoff changes. A nudge built on a compliance rate nobody measured
// would be a guess with a number on it.

// observeSubagentHandoff records one child run's closing protocol. Called from
// a single defer so every exit path is counted the same: a salvage, a review
// failure and a clean answer are all runs that either submitted a report or
// did not.
func observeSubagentHandoff(sink event.Sink, sub *Agent, sess *Session, opts Options, answer string, runErr error) {
	if sub == nil || sess == nil {
		return
	}
	audit := event.SubagentHandoffAudit{
		Entrance: opts.HandoffEntrance,
		Depth:    opts.SubagentDepth,
		ReadOnly: opts.ReadOnlyExecution,
		Expected: opts.ExpectCompletionReport,
		Exit:     handoffExit(answer, runErr),
	}
	countReportCalls(&audit, sess.Snapshot())
	// Claimed and adjudicated are both kept: a child that submits reports
	// readily and has them lowered every time is a different problem from one
	// that does not submit, and a single status cannot say which.
	if sub.task.ledger != nil {
		verdicts := sub.task.ledger.ClosureVerdicts()
		audit.Closed, audit.NeedsWork = verdicts.Closed, verdicts.NeedsWork
		if claimed, ok := sub.task.ledger.LatestCompletionReport(); ok {
			adjudicated, reasons := sub.task.ledger.AdjudicateCompletion(claimed)
			audit.ClaimedStatus = string(claimed.Status)
			audit.AdjudicatedStatus = string(adjudicated.Status)
			audit.LoweredClaims = len(reasons)
			audit.Criteria = len(adjudicated.Criteria)
			audit.Unresolved = len(adjudicated.Unresolved)
			for _, c := range adjudicated.Criteria {
				audit.Evidence += len(c.Evidence)
			}
		}
	}
	event.RecordSubagentHandoff(sink, audit)
}

// handoffExit classifies how the run ended. A provider failure that produced no
// report is not a compliance failure, and counting it as one would make the
// denominator answer a different question.
func handoffExit(answer string, runErr error) string {
	switch {
	case errors.Is(runErr, context.Canceled):
		return "cancelled"
	case runErr != nil:
		return "error"
	case strings.TrimSpace(answer) == "":
		return "no_answer"
	default:
		return "completed"
	}
}

// countReportCalls walks the child's transcript for complete_subtask calls and
// what the tool made of them. Attempted-and-refused is a schema problem;
// never-attempted is a protocol one, and only the counts can tell them apart.
func countReportCalls(audit *event.SubagentHandoffAudit, msgs []provider.Message) {
	calls := map[string]bool{}
	round := 0
	for _, m := range msgs {
		if len(m.ToolCalls) > 0 {
			round++
		}
		for _, tc := range m.ToolCalls {
			if tc.Name != completeSubtaskToolName {
				if audit.ReportRound > 0 {
					audit.ToolCallsAfterReport++
				}
				continue
			}
			calls[tc.ID] = true
			audit.Attempts++
			if audit.ReportRound == 0 {
				audit.ReportRound = round
			}
		}
		if m.Role != provider.RoleTool || !calls[m.ToolCallID] {
			continue
		}
		if isErrorMessage(m) {
			audit.Malformed++
			continue
		}
		audit.Accepted++
	}
	audit.FinalRound = round
}
