package agent

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// A fold's index gives an address; recall is what reads it. The budget is per
// generation so the fold that freed the window cannot be undone by the turns
// after it: a recall loop runs out of budget instead of running forever.
const recallBudgetRatio = 0.10

const minRecallBudgetTokens = 2000

// recallBudget is one generation's ceiling on pulling folded content back.
func (a *Agent) recallBudget() int {
	if a.contextWindow <= 0 {
		return minRecallBudgetTokens
	}
	return max(minRecallBudgetTokens, int(float64(a.contextWindow)*recallBudgetRatio))
}

// RecallContext returns folded canonical positions as text the caller can read.
// It never installs a projection: the recalled content re-enters context as an
// ordinary tool result, which puts it in the recent tail like any other work.
func (a *Agent) RecallContext(_ context.Context, req tool.RecallRequest) (tool.RecallResult, error) {
	if len(req.Positions) == 0 {
		return tool.RecallResult{}, fmt.Errorf("recall: no positions given")
	}
	canonical, _ := a.sess.conversation.snapshotMessagesVersion()

	a.sess.compactionMu.Lock()
	defer a.sess.compactionMu.Unlock()
	state := &a.sess.compactionState
	covered := state.Projection.CoveredCount
	if covered <= 0 {
		return tool.RecallResult{}, fmt.Errorf("recall: nothing has been folded in this session yet, so every position is still in your context")
	}
	if state.Recall.Generation != state.Generation {
		state.Recall = RecallLedger{Generation: state.Generation}
	}
	budget := a.recallBudget()
	left := budget - state.Recall.SpentTokens

	var body strings.Builder
	var recalled, missing []int
	for _, pos := range req.Positions {
		if pos < 0 || pos >= len(canonical) {
			missing = append(missing, pos)
			continue
		}
		if pos >= covered {
			return tool.RecallResult{}, fmt.Errorf("recall: #%d is not folded — it is still in your context, so read it there", pos)
		}
		rendered := renderTranscript(a.recallSpan(canonical, pos))
		if strings.TrimSpace(rendered) == "" {
			missing = append(missing, pos)
			continue
		}
		fmt.Fprintf(&body, "#%d\n%s\n", pos, strings.TrimRight(rendered, "\n"))
		recalled = append(recalled, pos)
	}
	if len(recalled) == 0 {
		return tool.RecallResult{BudgetLeft: left, Missing: missing},
			fmt.Errorf("recall: %s named nothing the transcript still holds", positionsText(missing))
	}
	// Refused whole rather than truncated: a half-recalled span reads as the
	// whole of what was there, which is the failure recall exists to prevent.
	cost := a.textTokens(body.String())
	if cost > left {
		return tool.RecallResult{BudgetLeft: left},
			fmt.Errorf("recall: %d tokens exceeds the %d left in this generation's recall budget — ask for fewer positions", cost, left)
	}
	state.Recall.SpentTokens += cost
	return tool.RecallResult{
		Text:       strings.TrimRight(body.String(), "\n"),
		Recalled:   recalled,
		Missing:    missing,
		Tokens:     cost,
		BudgetLeft: budget - state.Recall.SpentTokens,
	}, nil
}

// recallSpan returns the message at pos plus the tool results that answer it.
// An index line addresses the call, and a call without its result is the half
// that says least.
func (a *Agent) recallSpan(canonical []provider.Message, pos int) []provider.Message {
	span := []provider.Message{canonical[pos]}
	wanted := map[string]bool{}
	for _, tc := range canonical[pos].ToolCalls {
		wanted[tc.ID] = true
	}
	for i := pos + 1; i < len(canonical) && len(wanted) > 0; i++ {
		m := canonical[i]
		if m.Role != provider.RoleTool {
			break
		}
		if wanted[m.ToolCallID] {
			span = append(span, m)
			delete(wanted, m.ToolCallID)
		}
	}
	return span
}

func positionsText(positions []int) string {
	if len(positions) == 0 {
		return "the request"
	}
	parts := make([]string, len(positions))
	for i, p := range positions {
		parts[i] = fmt.Sprintf("#%d", p)
	}
	return strings.Join(parts, ", ")
}

var _ tool.ContextRecaller = (*Agent)(nil)
