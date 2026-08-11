package evidence

import "testing"

// drain settles the same kind of round until the account is empty and returns
// how many rounds that took.
func drain(r *Runway, s OutcomeSample) int {
	for rounds := 1; rounds <= 100; rounds++ {
		if r.Settle(s).Spent {
			return rounds
		}
	}
	return -1
}

// The rates are the whole design: producing nothing costs a full round,
// learning something costs half of one, and proving something earns rounds
// back. Nothing in here is a round counter — the same turn lasts longer or
// shorter purely by what it produces.
func TestRunwayDrainsAtTheRateTheTurnFailsToProduce(t *testing.T) {
	var repeating Runway
	if got := drain(&repeating, OutcomeSample{}); got != RunwayStart/runwayRoundCost {
		t.Errorf("rounds producing nothing lasted %d, want %d", got, RunwayStart/runwayRoundCost)
	}

	var looking Runway
	if got := drain(&looking, OutcomeSample{Exploration: 1}); got != RunwayStart {
		t.Errorf("look-only rounds lasted %d, want %d", got, RunwayStart)
	}

	// A landed change covers its own round: a turn that keeps changing things
	// is doing the work, and interrupting it at a round count is the false stop
	// this account exists to remove. Its unverified debt is the EBM trigger's
	// business, not the runway's.
	var changing Runway
	if got := drain(&changing, OutcomeSample{Churn: 1}); got != -1 {
		t.Errorf("a turn landing a change every round was cut off after %d rounds", got)
	}

	// A turn that keeps proving things never runs out.
	var proving Runway
	if got := drain(&proving, OutcomeSample{Discriminating: 1}); got != -1 {
		t.Errorf("a verifying turn was cut off after %d rounds", got)
	}
}

func TestRunwayRefillsAndIsBounded(t *testing.T) {
	var r Runway
	for range RunwayStart - 2 {
		r.Settle(OutcomeSample{Exploration: 1})
	}
	low := r.Settle(OutcomeSample{Exploration: 1})
	if !low.Low || low.Spent {
		t.Fatalf("state before the last rounds = %+v, want low but solvent", low)
	}
	if s := r.Settle(OutcomeSample{Discriminating: 1}); s.Low || s.Idle != 0 || s.Dry != 0 {
		t.Fatalf("a falsifiable observation left %+v, want the account restored", s)
	}

	// Banking is bounded: an hour of good work cannot buy unlimited silence.
	var banked Runway
	for range 100 {
		banked.Settle(OutcomeSample{Discriminating: 1})
	}
	if got := drain(&banked, OutcomeSample{Exploration: 1}); got != runwayCap {
		t.Errorf("a maximally productive turn banked %d look-only rounds, want the %d cap", got, runwayCap)
	}
}

// The observations the host states are counts of what it saw, not thresholds it
// crossed: a round that only looks is idle without being dry.
func TestRunwayReportsIdleAndDrySeparately(t *testing.T) {
	var r Runway
	var s RunwayState
	for range 3 {
		s = r.Settle(OutcomeSample{Exploration: 1})
	}
	if s.Idle != 3 || s.Dry != 0 {
		t.Fatalf("three look-only rounds = %+v, want idle 3 dry 0", s)
	}
	if s = r.Settle(OutcomeSample{}); s.Idle != 4 || s.Dry != 1 {
		t.Fatalf("a round producing nothing = %+v, want idle 4 dry 1", s)
	}
	if s = r.Settle(OutcomeSample{Churn: 1}); s.Idle != 0 || s.Dry != 0 {
		t.Fatalf("a change = %+v, want both counts cleared", s)
	}
}
