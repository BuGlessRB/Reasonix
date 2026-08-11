package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
	_ "reasonix/internal/tool/builtin"
)

func bashProgressReceipt(t *testing.T, command string, success bool) evidence.Receipt {
	t.Helper()
	args, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return evidence.ReceiptFromToolCall("bash", args, success, false)
}

// runRound executes one read-only batch and returns its result texts.
func runRound(t *testing.T, a *Agent, path string) []string {
	t.Helper()
	batch := a.executeBatch(context.Background(), []provider.ToolCall{
		{ID: "c", Name: "read_probe", Arguments: `{"path":"` + path + `"}`},
	})
	return batch.results
}

// A turn re-reading one path buys nothing and pays full price every round, so
// the account drains at its fastest rate. Nothing is said until the balance is
// worth stating, and the host speaks about its own state rather than issuing
// instructions.
func TestRunwayDrainsFastestOnRepeatedRounds(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_probe", readOnly: true})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)
	a.progress.reset()

	var spoke []int
	spentRound, roundsAfterSpent := 0, 3
	for round := 1; round <= evidence.RunwayStart+roundsAfterSpent; round++ {
		out := runRound(t, a, "same.go")[0]
		if strings.Contains(out, "[host]") {
			spoke = append(spoke, round)
			if strings.Contains(out, "runway is spent") {
				spentRound = round
			}
		}
		if spentRound == 0 && a.loopGuardArmed {
			t.Fatalf("readiness stood down at round %d, before the balance was spent", round)
		}
		if spentRound > 0 && round >= spentRound+roundsAfterSpent {
			break
		}
	}
	if spentRound == 0 {
		t.Fatal("a turn repeating one read must reach the end of its runway")
	}
	if len(spoke) == 0 || spoke[0] < 2 {
		t.Fatalf("host spoke at rounds %v; the opening balance must carry a turn silently for a while", spoke)
	}
	if got := spoke[len(spoke)-1]; got != spentRound {
		t.Fatalf("host last spoke at round %d but spent at %d; the transition is stated once, then silence", got, spentRound)
	}
	if !a.loopGuardArmed {
		t.Fatal("a spent runway must stand readiness down so the turn can answer")
	}
}

// The account is not a countdown: landing a falsifiable observation refills it,
// so a turn that keeps proving things is never cut off however long it runs.
func TestFalsifiableObservationRefillsTheRunway(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_probe", readOnly: true})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)
	a.progress.reset()

	for range 5 {
		runRound(t, a, "same.go")
	}
	drained := a.progress.runway
	a.evidence.Record(bashProgressReceipt(t, "go test ./pkg", true))
	mark := a.evidence.Len() - 1
	refilled := a.progress.runway.Settle(a.scoreRound(a.evidence.ReceiptsSince(mark)))
	if refilled.Low || refilled.Spent {
		t.Fatalf("a passing verification left the account at %+v, want it back above the low line", refilled)
	}
	if drained == a.progress.runway {
		t.Fatal("settling a verification round changed nothing")
	}
}

// Recovery closes the episode. A turn that spends its runway, earns it back and
// then stalls again must be told a second time — otherwise the first stand-down
// silences the host for the rest of the turn.
func TestASecondDrainSpeaksAgain(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_probe", readOnly: true})
	reg.Add(fakeTool{name: "bash"})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)
	a.resetTurnEvidence()

	verify := func() {
		a.executeBatch(context.Background(), []provider.ToolCall{
			{ID: "v", Name: "bash", Arguments: `{"command":"go test ./pkg"}`},
		})
	}
	spentRounds := 0
	drainToEmpty := func() {
		for range evidence.RunwayStart {
			out := runRound(t, a, "same.go")[0]
			if strings.Contains(out, "runway is spent") {
				spentRounds++
				return
			}
		}
	}

	drainToEmpty()
	if spentRounds != 1 {
		t.Fatalf("first drain reported %d spent transitions, want 1", spentRounds)
	}
	// Two falsifiable observations lift the account clear of the low band.
	verify()
	verify()
	a.loopGuardArmed = false
	drainToEmpty()
	if spentRounds != 2 {
		t.Fatalf("a recovered turn that stalled again reported %d spent transitions, want 2", spentRounds)
	}
	if !a.loopGuardArmed {
		t.Fatal("the second drain must stand readiness down again")
	}
}

// Without the balance on the round's sample a turn's pause cannot be explained
// from its trajectory, and the account's prices stay guesswork.
func TestEveryScoredRoundRecordsTheRunway(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_probe", readOnly: true})
	sink := &outcomeSampleSink{}
	a := New(nil, reg, NewSession(""), Options{}, sink)
	a.resetTurnEvidence()

	rounds := 0
	for range evidence.RunwayStart {
		runRound(t, a, "same.go")
		rounds++
		if sink.samples[len(sink.samples)-1].RunwaySpent {
			break
		}
	}
	if len(sink.samples) != rounds {
		t.Fatalf("got %d samples over %d rounds, want one per round", len(sink.samples), rounds)
	}
	first, last := sink.samples[0], sink.samples[len(sink.samples)-1]
	if first.Runway <= last.Runway {
		t.Errorf("balance did not fall across the turn: %d then %d", first.Runway, last.Runway)
	}
	if last.Runway != 0 || !last.RunwaySpent {
		t.Errorf("final sample = runway %d spent %v, want the spent transition recorded", last.Runway, last.RunwaySpent)
	}
	if last.RunwayDry == 0 || last.RunwayIdle == 0 {
		t.Errorf("final sample lost the counts behind the pause: %+v", last)
	}
}

type outcomeSampleSink struct {
	samples []evidence.OutcomeSample
}

func (s *outcomeSampleSink) Emit(event.Event) {}
func (s *outcomeSampleSink) RecordOutcomeProgress(sample evidence.OutcomeSample) {
	s.samples = append(s.samples, sample)
}

func TestOutcomeShadowRecordsEveryRoundWithoutTouchingGuards(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_probe", readOnly: true})
	sink := &outcomeSampleSink{}
	a := New(nil, reg, NewSession(""), Options{}, sink)
	a.resetTurnEvidence()

	first := runRound(t, a, "a.go")
	second := runRound(t, a, "a.go")
	if len(sink.samples) != 2 {
		t.Fatalf("got %d shadow samples, want one per round", len(sink.samples))
	}
	if s := sink.samples[0]; s.Round != 1 || s.Exploration != 1 || s.Objective != 0 {
		t.Fatalf("round 1 sample = %+v, want exploration 1 objective 0", s)
	}
	if s := sink.samples[1]; s.Round != 2 || s.Exploration != 0 {
		t.Fatalf("round 2 repeat sample = %+v, want no exploration", s)
	}
	// One sample per round feeds every policy, and while the account is solvent
	// no policy touches the round text.
	if strings.Contains(first[0], "[host]") || strings.Contains(second[0], "[host]") {
		t.Fatalf("a solvent turn must run untouched: %q / %q", first[0], second[0])
	}
}
