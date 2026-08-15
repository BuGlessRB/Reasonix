package agent

import (
	"testing"

	"reasonix/internal/provider"
)

// The keep budget is a share of the window, not a fixed token count. A user on
// a large window used to get the same 8192 as one on a small one, so most of
// their own words fell into the digest no matter how much room there was.
func TestUserTurnKeepBudgetScalesWithTheWindow(t *testing.T) {
	for _, tc := range []struct{ window, want int }{
		{20_000, 1_000},
		{128_000, 6_400},
		{1_000_000, 50_000},
	} {
		a := &Agent{}
		a.contextWindow = tc.window
		if got := a.keptUserTurnsBudget(); got != tc.want {
			t.Errorf("window %d: budget = %d, want %d", tc.window, got, tc.want)
		}
	}
}

// Every fold bound is the user's to name, and a set value is honoured exactly
// rather than clamped back to what the host would have picked.
func TestConfiguredCompactionBudgetsAreHonoured(t *testing.T) {
	a := &Agent{}
	a.contextWindow = 200_000
	a.budgets = CompactionBudgets{
		UserTurnKeepTokens: 40_000, FirstTurnPinTokens: 9_000, CheckpointCeilingRatio: 0.8,
	}
	if got := a.keptUserTurnsBudget(); got != 40_000 {
		t.Errorf("keep budget = %d, want the configured value", got)
	}
	if got := a.checkpointCeiling(); got != 160_000 {
		t.Errorf("checkpoint ceiling = %d, want 80%% of the window", got)
	}
	// The first-turn pin still answers to the window guard: it rides the fixed
	// prefix and is paid for on every request of the session.
	if !a.fixedPinnableUserTurn(provider.Message{Role: provider.RoleUser, Content: shortText(8_000)}) {
		t.Error("a turn inside the configured pin budget was refused")
	}
	huge := &Agent{}
	huge.contextWindow = 20_000
	huge.budgets = CompactionBudgets{FirstTurnPinTokens: 1_000_000}
	if huge.fixedPinnableUserTurn(provider.Message{Role: provider.RoleUser, Content: shortText(40_000)}) {
		t.Error("the window guard must still bound the pinned first turn")
	}
}

// Unset means the built-in default, so an untouched config behaves as before.
func TestUnsetCompactionBudgetsKeepTheDefaults(t *testing.T) {
	a := &Agent{}
	a.contextWindow = 200_000
	if got, want := a.checkpointCeiling(), 100_000; got != want {
		t.Errorf("checkpoint ceiling = %d, want the default half-window %d", got, want)
	}
	if got, want := a.keptUserTurnsBudget(), 10_000; got != want {
		t.Errorf("keep budget = %d, want the default window share %d", got, want)
	}
}

func shortText(chars int) string {
	b := make([]byte, chars)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
