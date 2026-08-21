package agent

import (
	"strings"
	"testing"

	"reasonix/internal/tool"
)

// gen names one model-visible history build; a fold moves the projection.
func gen(projection uint64) ContextGeneration {
	return ContextGeneration{Projection: projection}
}

func budgetAt(used, compactAt, window int) tool.ContextBudget {
	return tool.ContextBudget{
		Status:          "ok",
		TokensRemaining: max(0, compactAt-used),
		TokensUsed:      used,
		CompactAt:       compactAt,
		Window:          window,
	}
}

func TestContextBudgetRungCrossings(t *testing.T) {
	const compactAt = 10_000
	cases := []struct {
		used int
		want int
	}{
		{0, 0},
		{7_499, 0},
		{7_500, 1}, // 0.75
		{9_199, 1},
		{9_200, 2}, // 0.92
		{20_000, 2},
	}
	for _, tc := range cases {
		if got := contextBudgetRung(budgetAt(tc.used, compactAt, 12_000)); got != tc.want {
			t.Fatalf("used %d: rung = %d, want %d", tc.used, got, tc.want)
		}
	}
}

// A rung must fire exactly once. The notice occupies the context it reports on,
// so a re-fire on every step would spend the remaining window describing it.
func TestBudgetNoticeFiresOncePerRung(t *testing.T) {
	var latch budgetNoticeLatch
	notice, latch := advanceBudgetNotice(latch, budgetAt(7_600, 10_000, 12_000), gen(3))
	if notice == "" {
		t.Fatal("first crossing produced no notice")
	}
	if !strings.Contains(notice, "<context-budget>") {
		t.Fatalf("notice is not a tagged fragment: %q", notice)
	}
	for _, used := range []int{7_700, 8_000, 9_000} {
		var again string
		again, latch = advanceBudgetNotice(latch, budgetAt(used, 10_000, 12_000), gen(3))
		if again != "" {
			t.Fatalf("used %d re-fired the same rung: %q", used, again)
		}
	}
	notice, latch = advanceBudgetNotice(latch, budgetAt(9_500, 10_000, 12_000), gen(3))
	if notice == "" {
		t.Fatal("second rung did not fire")
	}
	if latch.rung != 2 {
		t.Fatalf("latch.rung = %d, want 2", latch.rung)
	}
}

// A fold drops usage and starts a new history build. Every rung must re-arm:
// the model that just survived a compaction has not been told about the next one.
func TestBudgetNoticeReArmsAfterFold(t *testing.T) {
	var latch budgetNoticeLatch
	_, latch = advanceBudgetNotice(latch, budgetAt(9_500, 10_000, 12_000), gen(1))
	if latch.rung != 2 {
		t.Fatalf("setup: latch.rung = %d, want 2", latch.rung)
	}
	notice, latch := advanceBudgetNotice(latch, budgetAt(7_600, 10_000, 12_000), gen(2))
	if notice == "" {
		t.Fatal("rung 1 did not re-arm after the history build changed")
	}
	if latch.rung != 1 || latch.generation != gen(2) {
		t.Fatalf("latch = %+v, want rung 1 at generation 2", latch)
	}
}

// An unmeasured budget must stay silent rather than report zero room left.
func TestBudgetNoticeSilentWhenUnmeasured(t *testing.T) {
	notice, latch := advanceBudgetNotice(budgetNoticeLatch{}, unmeasuredContextBudget("no window"), gen(1))
	if notice != "" {
		t.Fatalf("unmeasured budget produced a notice: %q", notice)
	}
	if latch != (budgetNoticeLatch{}) {
		t.Fatalf("unmeasured budget moved the latch: %+v", latch)
	}
}

func TestContextBudgetUnmeasuredWithoutWindow(t *testing.T) {
	a := &Agent{agentConfig: agentConfig{contextWindow: 0, compactRatio: defaultCompactRatio}}
	budget := a.ContextBudget()
	if budget.Known() {
		t.Fatalf("budget reported as known without a window: %+v", budget)
	}
	if budget.Status != "unmeasured" || budget.Reason == "" {
		t.Fatalf("unmeasured budget must name a reason: %+v", budget)
	}
}
