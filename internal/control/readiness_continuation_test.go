package control

import (
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
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
