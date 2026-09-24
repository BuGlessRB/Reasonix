// subagent_handoff_shadow.go — how often a child closes the way it was asked to.
package agent

import (
	"context"
	"errors"
	"strings"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

// Nothing here decides anything: no prompt byte moves, no notice is emitted, no
// parent handoff changes. A nudge built on a compliance rate nobody measured
// would be a guess with a number on it.

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
