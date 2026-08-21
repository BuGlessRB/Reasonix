package agent

import (
	"fmt"

	"reasonix/internal/tool"
)

// contextBudgetNoticeRatios are the fractions of the distance to the compaction
// trigger at which the host volunteers the remaining budget. Two rungs, not a
// gauge: the notice occupies the very context it reports on, so it earns a turn
// only where the model's next decision changes — once with room to redirect,
// once with room only to land what it already knows.
var contextBudgetNoticeRatios = [...]float64{0.75, 0.92}

// ContextBudget reports the room left before the next automatic fold, measured
// with the estimate the compaction thresholds themselves compare against so
// what the model is told and what the host acts on cannot drift apart.
func (a *Agent) ContextBudget() tool.ContextBudget {
	if a == nil {
		return unmeasuredContextBudget("no active agent session")
	}
	trigger := a.compactTrigger()
	window := a.effectiveContextWindow()
	if trigger <= 0 || window <= 0 {
		return unmeasuredContextBudget("the active provider declares no context window, so compaction is disabled")
	}
	used := a.ContextUsedTokens()
	return tool.ContextBudget{
		Status:          "ok",
		TokensRemaining: max(0, trigger-used),
		TokensUsed:      used,
		CompactAt:       trigger,
		Window:          window,
	}
}

func unmeasuredContextBudget(reason string) tool.ContextBudget {
	return tool.ContextBudget{Status: "unmeasured", Reason: reason}
}

// budgetNoticeLatch remembers how far up the notice ladder this conversation has
// already been told, under the history build that makes it true. A fold drops
// usage and starts a new build, which re-arms every rung: the model about to be
// compacted and the model that just was need different things, and only the
// generation tells them apart.
type budgetNoticeLatch struct {
	rung       int
	generation ContextGeneration
}

// contextBudgetNotice returns the message to append before the next sampling
// call, or "" when the model has already been told what current pressure
// warrants. It advances the latch, so each rung fires once per fold.
func (a *Agent) contextBudgetNotice() string {
	if a == nil {
		return ""
	}
	if a.Session() == nil {
		return ""
	}
	notice, latch := advanceBudgetNotice(a.sess.budgetNotice, a.ContextBudget(), a.ContextGeneration())
	a.sess.budgetNotice = latch
	return notice
}

// advanceBudgetNotice decides what the current pressure warrants and returns the
// latch to store. Split from contextBudgetNotice so the edge-trigger rule is
// testable without a live session's token estimator.
func advanceBudgetNotice(latch budgetNoticeLatch, budget tool.ContextBudget, generation ContextGeneration) (string, budgetNoticeLatch) {
	if !budget.Known() {
		return "", latch
	}
	if latch.generation != generation {
		latch = budgetNoticeLatch{generation: generation}
	}
	rung := contextBudgetRung(budget)
	if rung <= latch.rung {
		return "", latch
	}
	latch.rung = rung
	return contextBudgetNoticeText(budget, rung), latch
}

// contextBudgetRung is the highest threshold the current usage has crossed;
// 0 means the conversation is still below the first one.
func contextBudgetRung(budget tool.ContextBudget) int {
	rung := 0
	for i, ratio := range contextBudgetNoticeRatios {
		if float64(budget.TokensUsed) >= float64(budget.CompactAt)*ratio {
			rung = i + 1
		}
	}
	return rung
}

func contextBudgetNoticeText(budget tool.ContextBudget, rung int) string {
	if rung >= len(contextBudgetNoticeRatios) {
		return fmt.Sprintf(`<context-budget>
About %d tokens of room remain before this conversation is automatically compacted.
Land what you know now: state the current result, the exact next step, and any path, identifier, or number the summary would otherwise have to carry for you.
</context-budget>`, budget.TokensRemaining)
	}
	return fmt.Sprintf(`<context-budget>
About %d tokens of room remain before this conversation is automatically compacted (%d used of a %d-token window; the fold triggers at %d).
Compaction folds earlier assistant and tool messages into a summary. The user's own turns stay verbatim, but anything you are holding only in your own earlier replies — exact paths, line numbers, a half-finished plan — survives only if you restate it or put it in the todo list.
Work narrower from here: scope searches, read ranges rather than whole files, and do not start work whose output you cannot finish reading. Call context_budget when you need the current figure.
</context-budget>`, budget.TokensRemaining, budget.TokensUsed, budget.Window, budget.CompactAt)
}

// contextBudgetNoticeSummary is the user-facing one-liner for the same event.
// The model gets the instructions; the user gets to see that it was told.
func contextBudgetNoticeSummary(budget tool.ContextBudget) string {
	if !budget.Known() {
		return "Context budget unmeasured; the model was not notified."
	}
	return fmt.Sprintf("Context at %d%% of the compaction threshold — the model was told it has about %d tokens of room left.",
		int(float64(budget.TokensUsed)/float64(budget.CompactAt)*100), budget.TokensRemaining)
}
