package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func planTodos() []evidence.TodoItem {
	return []evidence.TodoItem{
		{Content: "Wire the parser", Status: "completed", StepID: "plan_step_01"},
		{Content: "Add the tests", Status: "in_progress", StepID: "plan_step_02"},
		{Content: "Ship it", Status: "pending", StepID: "plan_step_03"},
	}
}

func TestTodoCitationPrefersTheStableID(t *testing.T) {
	if got := evidence.TodoCitation("plan_step_02", 2, "Add the tests"); got != "[plan_step_02] Add the tests" {
		t.Fatalf("citation = %q, want the stable id", got)
	}
	// A list that never carried ids still has to name its items somehow.
	if got := evidence.TodoCitation("", 2, "Add the tests"); got != "2) Add the tests" {
		t.Fatalf("citation = %q, want the ordinal fallback", got)
	}
}

func TestProjectionRestatesTheIDsAfterAFoldTakesThemOutOfView(t *testing.T) {
	a := New(&scriptedProvider{name: "p"}, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	a.ReplaceTodoState(planTodos())
	a.noteTodoIdentityShown() // the model wrote this list and can still read it

	if got := a.todoIdentityProjection(); got != "" {
		t.Fatalf("projection = %q, want none while the ids are still visible", got)
	}

	a.noteTodoIdentityLost() // a fold replaced the region holding them

	got := a.todoIdentityProjection()
	for _, id := range []string{"plan_step_01", "plan_step_02", "plan_step_03"} {
		if !strings.Contains(got, "["+id+"]") {
			t.Fatalf("projection = %q, want it to carry %s", got, id)
		}
	}
	if second := a.todoIdentityProjection(); second != "" {
		t.Fatalf("projection repeated as %q; it is owed once per loss, not every turn", second)
	}
}

func TestNoProjectionWithoutStableIDs(t *testing.T) {
	a := New(&scriptedProvider{name: "p"}, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	a.ReplaceTodoState([]evidence.TodoItem{{Content: "Do the thing", Status: "in_progress"}})
	a.noteTodoIdentityLost()

	// Nothing to re-project: an ordinal is not an identity the host owns.
	if got := a.todoIdentityProjection(); got != "" {
		t.Fatalf("projection = %q, want none for a list with no ids", got)
	}
}

// TestTodoWriteResultNamesTheSignableStepID is the renderer half: the model is
// asked for a step_id, so a step_id has to come back with the list it wrote.
func TestTodoWriteResultNamesTheSignableStepID(t *testing.T) {
	todoWrite, ok := tool.LookupBuiltin("todo_write")
	if !ok {
		t.Fatal("todo_write builtin not registered")
	}
	reg := tool.NewRegistry()
	reg.Add(todoWrite)

	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{toolCallChunk("c1", "todo_write", `{"todos":[
			{"content":"Wire the parser","status":"in_progress","activeForm":"Wiring the parser","step_id":"plan_step_01"},
			{"content":"Add the tests","status":"pending","activeForm":"Adding the tests","step_id":"plan_step_02"}
		]}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
	}}

	a := New(prov, reg, NewSession(""), Options{}, event.Discard)
	if err := a.Run(context.Background(), "plan the work"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := toolResult(a.sess.conversation, "todo_write")
	if !strings.Contains(got, "plan_step_01") {
		t.Fatalf("todo_write result = %q, want the in_progress item's step_id", got)
	}
}

func TestFoldPathAppendsTheIDsOnlyWhenItsOwnOutputLostThem(t *testing.T) {
	a := New(&scriptedProvider{name: "p"}, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	a.ReplaceTodoState(planTodos())

	folded := []provider.Message{{Role: provider.RoleUser, Content: "summary of earlier work"}}
	got := a.withTodoIdentityProjection(folded)
	if len(got) != 2 || !strings.Contains(got[1].Content, "[plan_step_02]") {
		t.Fatalf("projection = %+v, want the ids appended to a fold that dropped them", got)
	}

	// A fold whose kept tail still carries the ids owes nothing: this replaces
	// an identity that was lost, it does not restate one that survived.
	kept := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		Name:      "todo_write",
		Arguments: `{"todos":[{"step_id":"plan_step_01"},{"step_id":"plan_step_02"},{"step_id":"plan_step_03"}]}`,
	}}}}
	if got := a.withTodoIdentityProjection(kept); len(got) != 1 {
		t.Fatalf("projection appended %d messages, want none while the ids are still readable", len(got)-1)
	}
}

// TestRealFoldLeavesTheStepIDsReadable drives actual compaction and asserts on
// the installed projection: the round the fold feeds is already frozen when the
// next turn starts, so an id restored a round later is one sign-off too late.
func TestRealFoldLeavesTheStepIDsReadable(t *testing.T) {
	mock := &loopMock{t: t}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	reg := tool.NewRegistry()
	reg.Add(fatTool{blob: strings.Repeat("file line. ", 1100)})
	a, _ := newAgent(t, srv.URL, reg, 40000, 4)
	a.ReplaceTodoState(planTodos())
	a.noteTodoIdentityShown()

	for i := range 20 {
		if err := a.Run(context.Background(), fmt.Sprintf("turn %d: keep going", i)); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}

	a.sess.compactionMu.Lock()
	projected := a.sess.compactionState.Projection.Messages
	a.sess.compactionMu.Unlock()
	if len(projected) == 0 {
		t.Fatal("no fold was installed; this test asserts nothing without one")
	}
	for _, id := range []string{"plan_step_01", "plan_step_02", "plan_step_03"} {
		if !messagesMentionID(projected, id) {
			t.Fatalf("the installed projection dropped %s; the fold's own request cannot cite it", id)
		}
	}
}
