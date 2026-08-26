package control

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// readinessDeliveryController wires a Delivery agent to a scripted provider, so
// the ledger the readiness check reads is the real one the turns produced.
func readinessDeliveryController(t *testing.T, prov provider.Provider) (*Controller, chan event.Event) {
	t.Helper()
	todoWrite, _ := tool.LookupBuiltin("todo_write")
	reg := tool.NewRegistry()
	reg.Add(todoWrite)
	// complete_step is what advances the list; without it in the registry these
	// turns exercise a run that can never satisfy a todo requirement at all.
	if completeStep, ok := tool.LookupBuiltin("complete_step"); ok {
		reg.Add(completeStep)
	}
	reg.Add(minimalFakeTool{name: "write_file"})
	reg.Add(minimalFakeTool{name: "bash"})
	ag := agent.New(prov, reg, agent.NewSession(""), agent.Options{DeliveryProfile: true}, event.Discard)
	done := make(chan event.Event, 4)
	c := New(Options{
		Runner: ag, Executor: ag,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.TurnDone {
				done <- e
			}
		}),
	})
	t.Cleanup(c.Close)
	return c, done
}

// The host read the missing requirements off its own receipts. Handing the user
// a card that only says "continue" asks them to relay a message the host wrote,
// so the work continues on its own instead.
func TestOrdinaryTurnFinishesWhatItOwes(t *testing.T) {
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		{toolCallChunk("t0", "todo_write", `{"todos":[{"content":"Ship main","status":"in_progress"}]}`), {Type: provider.ChunkDone}},
		{toolCallChunk("w1", "write_file", `{"path":"main.go"}`), {Type: provider.ChunkDone}},
		textTurn("premature final"),
		// The continuation does the work the host said was missing.
		{toolCallChunk("b1", "bash", `{"command":"go test ./..."}`), {Type: provider.ChunkDone}},
		{toolCallChunk("t1", "todo_write", `{"todos":[{"content":"Ship main","status":"completed"}]}`), {Type: provider.ChunkDone}},
		{toolCallChunk("c1", "complete_step", `{"step":"Ship main","result":"shipped","evidence":[{"kind":"verification","summary":"go test","command":"go test ./..."}]}`), {Type: provider.ChunkDone}},
		textTurn("done and verified"),
	}}
	c, done := readinessDeliveryController(t, prov)

	c.Submit("implement main")
	ev := <-done

	if prov.call <= 3 {
		t.Fatalf("provider calls = %d, want the turn to continue past its own premature final", prov.call)
	}
	// The continuation inherits the finished turn's receipts: a run starting
	// from an empty ledger would owe nothing at all, and the gap would read as
	// closed because the record of it was dropped.
	if ev.Readiness != nil && len(ev.Readiness.Missing) == 0 {
		t.Fatal("the continuation reported an empty gap, which is what a dropped ledger looks like")
	}
}

// A round that owes what the last one owed is not progress. Two of those hand
// the turn back rather than repeating forever.
func TestStalledReadinessStopsInsteadOfLooping(t *testing.T) {
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		{toolCallChunk("t0", "todo_write", `{"todos":[{"content":"Ship main","status":"in_progress"}]}`), {Type: provider.ChunkDone}},
		{toolCallChunk("w1", "write_file", `{"path":"main.go"}`), {Type: provider.ChunkDone}},
		// Every later turn answers without doing anything, so the gap never moves.
		textTurn("still not verified"),
	}}
	c, done := readinessDeliveryController(t, prov)

	c.Submit("implement main")
	ev := <-done

	if ev.Readiness == nil || len(ev.Readiness.Missing) == 0 {
		t.Fatalf("TurnDone.Readiness = %+v, want the unmet requirements handed back", ev.Readiness)
	}
	// One work turn, one premature final, then the bounded continuations.
	if prov.call > 6 {
		t.Fatalf("provider calls = %d, want the stall to stop the continuations", prov.call)
	}
}

// A turn whose remaining items should no longer be done needs something to
// answer with. When the only instruction was "finish it now", asking the run to
// stop changed nothing: the same requirement came back every round and the user
// was left holding the stop button.
func TestContinuationOffersAWayOutBesidesFinishing(t *testing.T) {
	prompt := readinessContinuationPrompt([]evidence.TodoItem{
		{Content: "Ship main", Status: "in_progress"},
	}, "the check you ran did not pass")
	// complete_step is the only action that advances the list, so a demand that
	// does not name it describes a door the guard keeps shut.
	for _, want := range []string{"complete_step", "todo_write", "conclude_blocked"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("continuation names no %s exit, so the only answer is to keep going:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "Ship main") {
		t.Errorf("continuation does not say what is outstanding:\n%s", prompt)
	}
}

// A run works through its list one sign-off at a time. Two things used to stop
// it: the gap was counted by category, so closing an item looked like closing
// nothing, and the instruction said "finish it" while the completion guard
// rejected the only action that phrase describes. Neither the list nor the
// demand ever moved, and the user was left holding the stop button.
func TestRunWorksThroughItsListInsteadOfRepeatingItself(t *testing.T) {
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		{toolCallChunk("t0", "todo_write", `{"todos":[{"content":"A","status":"in_progress"},{"content":"B","status":"pending"},{"content":"C","status":"pending"}]}`), {Type: provider.ChunkDone}},
		{toolCallChunk("w1", "write_file", `{"path":"main.go"}`), {Type: provider.ChunkDone}},
		textTurn("premature final"),
		// complete_step advances the list; a todo_write marking A done is refused.
		{toolCallChunk("b1", "bash", `{"command":"go test ./..."}`), {Type: provider.ChunkDone}},
		{toolCallChunk("c1", "complete_step", `{"step":"A","result":"done","evidence":[{"kind":"verification","summary":"go test","command":"go test ./..."}]}`), {Type: provider.ChunkDone}},
		textTurn("A done"),
		{toolCallChunk("c2", "complete_step", `{"step":"B","result":"done","evidence":[{"kind":"verification","summary":"go test","command":"go test ./..."}]}`), {Type: provider.ChunkDone}},
		textTurn("B done"),
		{toolCallChunk("c3", "complete_step", `{"step":"C","result":"done","evidence":[{"kind":"verification","summary":"go test","command":"go test ./..."}]}`), {Type: provider.ChunkDone}},
		textTurn("all three done and verified"),
	}}
	c, done := readinessDeliveryController(t, prov)

	c.Submit("do A, B and C")
	ev := <-done

	// Counting categories stalls after the second identical list, around call 7,
	// with B and C never reached.
	if prov.call < 9 {
		t.Fatalf("provider calls = %d: the run stopped while it was still signing items off (readiness=%+v)", prov.call, ev.Readiness)
	}
}

// treadmillTurns keeps the unmet-requirement state changing forever: every
// round adds one todo and finishes none, so no round is ever identical to the
// last. Progress-shaped and endless is exactly the case a stall counter cannot
// see.
type treadmillTurns struct {
	call int
}

func (t *treadmillTurns) Name() string { return "treadmill" }

func (t *treadmillTurns) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	t.call++
	ch := make(chan provider.Chunk, 2)
	switch {
	case t.call == 1:
		ch <- toolCallChunk("w0", "write_file", `{"path":"main.go"}`)
	case t.call%2 == 0:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "still going"}
	default:
		var items []string
		for i := range t.call {
			items = append(items, fmt.Sprintf(`{"content":"item %d","status":"pending"}`, i))
		}
		ch <- toolCallChunk(fmt.Sprintf("t%d", t.call), "todo_write",
			`{"todos":[`+strings.Join(items, ",")+`]}`)
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// A run whose gap never repeats itself is still a run that has to end. Without
// a ceiling the continuation only stops when two rounds look identical, which a
// list that grows as fast as it is worked never does — and the user is left
// pressing stop.
func TestContinuationEndsEvenWhenTheGapKeepsChanging(t *testing.T) {
	prov := &treadmillTurns{}
	c, done := readinessDeliveryController(t, prov)

	c.Submit("do the work")
	<-done

	if prov.call > 2*(readinessMaxRounds+2) {
		t.Fatalf("provider calls = %d: the continuation kept running on a gap that only ever changed shape", prov.call)
	}
}

// ask and conclude_blocked are the two ways a turn ends on the model's terms
// rather than the host's, and the host's own account of the ways out named only
// the second. The unnamed one is exactly the exit for a decision the user has to
// make, so a turn that stopped to put a question to them was continued straight
// past it and the run answered the question itself.
func TestContinuationNamesBothTurnExits(t *testing.T) {
	prompt := readinessContinuationPrompt([]evidence.TodoItem{
		{Content: "Write the spec once the shape is confirmed", Status: "in_progress"},
	}, "")
	// \bask\b, because "tasks" contains the tool's name and proves nothing.
	if !regexp.MustCompile(`\bask\b`).MatchString(prompt) {
		t.Errorf("continuation never names the ask exit, so a turn waiting on the user can only guess:\n%s", prompt)
	}
	if !strings.Contains(prompt, "conclude_blocked") {
		t.Errorf("continuation never names the conclude_blocked exit:\n%s", prompt)
	}
}
