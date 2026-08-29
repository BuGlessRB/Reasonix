package control

import (
	"path/filepath"
	"slices"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/goaleval"
	"reasonix/internal/instruction"
	"reasonix/internal/provider"
	"reasonix/internal/testenv"
)

// Can a real lifecycle produce a state where the criterion a task is held to
// and the declaration the gate reads are different commands? Nothing here
// assigns projectChecks, the checkpoint or the baseline: the two agents are
// built the way two boots build them, from two different declarations, and the
// restore is the controller's own resume path.
func TestGoalResumeRestoresProjectCheckBaselineAcrossRestart(t *testing.T) {
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "session.jsonl")
	const checkA, checkB = "python3 check_a.py", "python3 check_b.py"
	declares := func(command string) agent.Options {
		return agent.Options{ProjectChecks: []instruction.VerifyCheck{
			{Command: command, SourcePath: "REASONIX.md", Line: 5},
		}}
	}

	// Process one reads the declaration naming A and runs one goal turn.
	first := agent.New(&scriptedTurns{turns: [][]provider.Chunk{textTurn("done")}},
		goalRegistry(), agent.NewSession(""), declares(checkA), event.Discard)
	events := make(chan event.Event, 8)
	c1 := New(Options{
		Runner: first, Executor: first, SessionDir: dir, SessionPath: path, Label: "first",
		GoalEvaluator: &fakeGoalEvaluator{outcome: goaleval.OutcomeComplete, reason: "done"},
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.TurnDone || e.Kind == event.Notice {
				events <- e
			}
		}),
	})
	c1.Submit("/goal do the work")
	waitGoalTurnDone(t, events)

	wantA := evidence.VerificationIdentity(checkA)
	if got := first.DeliveryCheckpoint().BaselineChecks; !slices.Contains(got, wantA) {
		t.Fatalf("baseline after the first goal turn = %v, want the declared criterion %q captured", got, wantA)
	}

	// The declaration changes between the two processes. Process two reads it,
	// which is the only way its check list can differ from the first's.
	second := agent.New(&scriptedTurns{turns: [][]provider.Chunk{textTurn("done")}},
		goalRegistry(), agent.NewSession(""), declares(checkB), event.Discard)
	c2 := New(Options{Runner: second, Executor: second, SessionDir: dir, SessionPath: path, Label: "second"})
	c2.resume(agent.NewSession(""), path, false)

	restored := second.DeliveryCheckpoint().BaselineChecks
	wantB := evidence.VerificationIdentity(checkB)
	switch {
	case slices.Contains(restored, wantA):
		t.Logf("REACHABLE: the resumed goal holds %q while the declaration now names %q", wantA, wantB)
	case len(restored) == 0:
		t.Fatalf("the resumed goal restored no baseline at all: the sidecar carried none, "+
			"so the divergent state has no producer here either (restored=%v)", restored)
	default:
		t.Fatalf("the resumed goal holds %v, not the criterion the task began under (%q): "+
			"the baseline was recaptured from the declaration this process read, "+
			"so baseline and current cannot differ", restored, wantA)
	}
}
