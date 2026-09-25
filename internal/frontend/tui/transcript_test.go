package tui

import (
	"testing"

	"reasonix/internal/contract/eventwire"
)

func fold(evs ...eventwire.Event) *Transcript {
	t := &Transcript{}
	for _, ev := range evs {
		t.Apply(ev)
	}
	return t
}

func kinds(t *Transcript) []ItemKind {
	out := make([]ItemKind, len(t.Items))
	for i, it := range t.Items {
		out[i] = it.Kind
	}
	return out
}

// The settled message is the record: it repairs a stream that lost a chunk,
// and it is the whole answer when the deltas never came.
func TestMessageSettlesTheStreamedAnswer(t *testing.T) {
	tr := fold(
		eventwire.Event{Kind: "turn_started"},
		eventwire.Event{Kind: "reasoning", Text: "think"},
		eventwire.Event{Kind: "text", Text: "Hel"},
		eventwire.Event{Kind: "text", Text: "o"},
		eventwire.Event{Kind: "message", Text: "Hello", ThoughtMs: 1200},
	)
	if len(tr.Items) != 1 || tr.Items[0].Text != "Hello" || tr.Items[0].Reasoning != "think" || !tr.Items[0].Done || tr.Items[0].ThoughtMs != 1200 {
		t.Fatalf("items = %+v", tr.Items)
	}
	rebuilt := fold(eventwire.Event{Kind: "message", Text: "from the record"})
	if len(rebuilt.Items) != 1 || rebuilt.Items[0].Text != "from the record" || !rebuilt.Items[0].Done {
		t.Fatalf("message with no deltas = %+v", rebuilt.Items)
	}
	if empty := fold(eventwire.Event{Kind: "message"}); len(empty.Items) != 0 {
		t.Fatalf("an empty message made a row: %+v", empty.Items)
	}
}

// One call is one card: its later frames fill it in rather than stacking, and a
// sub-agent's calls fold under the task that spawned them.
func TestToolFramesFoldIntoOneCard(t *testing.T) {
	tr := fold(
		eventwire.Event{Kind: "text", Text: "let me look"},
		eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "t1", Name: "bash", Args: `{"command":"ls"}`}},
		eventwire.Event{Kind: "tool_progress", Tool: &eventwire.Tool{ID: "t1", Output: "a"}},
		eventwire.Event{Kind: "tool_result", Tool: &eventwire.Tool{ID: "t1", Name: "bash", Output: "a\nb", DurationMs: 30}},
		eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "task1", Name: "task"}},
		eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "c1", Name: "read_file", ParentID: "task1"}},
		eventwire.Event{Kind: "tool_result", Tool: &eventwire.Tool{ID: "c1", Output: "x", ParentID: "task1"}},
	)
	if got := kinds(tr); len(got) != 3 || got[0] != ItemSay || got[1] != ItemTool || got[2] != ItemTool {
		t.Fatalf("kinds = %v", got)
	}
	if !tr.Items[0].Done {
		t.Fatal("the answer before a call stayed open")
	}
	bash := tr.Items[1]
	if bash.Running || bash.Tool.Args != `{"command":"ls"}` || bash.Tool.Output != "a\nb" || bash.Tool.DurationMs != 30 {
		t.Fatalf("bash card = %+v %+v", bash, bash.Tool)
	}
	task := tr.Items[2]
	if len(task.Children) != 1 || task.Children[0].Output != "x" || task.Children[0].Name != "read_file" {
		t.Fatalf("task children = %+v", task.Children)
	}
}

// An approval stays open until this screen answers it or the kernel says it
// was answered elsewhere; the receipt's own notice never becomes a row.
func TestPromptsSettleHereOrElsewhere(t *testing.T) {
	tr := fold(
		eventwire.Event{Kind: "approval_request", Approval: &eventwire.Approval{ID: "a1", Tool: "bash", Subject: "rm -rf build"}},
		eventwire.Event{Kind: "ask_request", Ask: &eventwire.Ask{ID: "q1"}},
	)
	open := tr.OpenPrompt()
	if open == nil || open.Kind != ItemAsk {
		t.Fatalf("open prompt = %+v", open)
	}
	tr.Decide(open.ID, "answered")
	if open := tr.OpenPrompt(); open == nil || open.Approval.ID != "a1" {
		t.Fatalf("after answering the ask, open = %+v", open)
	}
	tr.Apply(eventwire.Event{Kind: "notice", Code: "decision_receipt", Text: "Decision recorded: allow",
		DecisionReceipt: &eventwire.DecisionReceipt{ID: "a1", Outcome: "allow"}})
	if tr.OpenPrompt() != nil {
		t.Fatal("an approval answered elsewhere stayed open")
	}
	if tr.Items[0].Verdict != "elsewhere" || len(tr.Items) != 2 {
		t.Fatalf("items = %+v", tr.Items)
	}
}

// Input handed to a running turn stays pending until the turn reads it, and
// then sits where it was read, not where it was typed.
func TestSteerMovesPendingInputToWhereItWasRead(t *testing.T) {
	tr := &Transcript{}
	tr.AddUser("start", false, "")
	tr.AddUser("also check tests", true, "q-1")
	tr.Apply(eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "t1", Name: "bash"}})
	tr.Apply(eventwire.Event{Kind: "steer", Text: "also check tests", ItemID: "q-1"})
	last := tr.Items[len(tr.Items)-1]
	if last.Kind != ItemUser || last.Pending || !last.Steer || last.Text != "also check tests" {
		t.Fatalf("items = %+v", tr.Items)
	}
	if len(tr.Items) != 3 {
		t.Fatalf("steer duplicated the row: %+v", tr.Items)
	}
}

func TestNoticesFoldRepeatsAndSkipOperatorInfo(t *testing.T) {
	tr := fold(
		eventwire.Event{Kind: "notice", Level: "warn", Code: "retry", Text: "retrying"},
		eventwire.Event{Kind: "notice", Level: "warn", Code: "retry", Text: "retrying"},
		eventwire.Event{Kind: "notice", Level: "info", Audience: "operator", Text: "model picked"},
	)
	if len(tr.Items) != 1 || tr.Items[0].Count != 2 {
		t.Fatalf("items = %+v", tr.Items)
	}
}

// How a turn ended is four answers, not two, and the end seals what it left
// open: an answer still streaming, a call still drawn as running.
func TestTurnDoneSealsAndSaysHowItEnded(t *testing.T) {
	cases := []struct {
		ev   eventwire.Event
		want Terminal
	}{
		{eventwire.Event{Kind: "turn_done"}, TurnCompleted},
		{eventwire.Event{Kind: "turn_done", Cancelled: true, Err: "cancelled"}, TurnCancelled},
		{eventwire.Event{Kind: "turn_done", Err: "provider down"}, TurnFailed},
		{eventwire.Event{Kind: "turn_done", Outcome: "unverified"}, TurnIncomplete},
	}
	for _, c := range cases {
		tr := fold(
			eventwire.Event{Kind: "turn_started"},
			eventwire.Event{Kind: "text", Text: "partial"},
			eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "t1", Name: "bash"}},
			c.ev,
		)
		if tr.Running || tr.Terminal != c.want {
			t.Fatalf("%+v: running=%v terminal=%v", c.ev, tr.Running, tr.Terminal)
		}
		if !tr.Items[0].Done || tr.Items[1].Running {
			t.Fatalf("%+v left something open: %+v", c.ev, tr.Items)
		}
	}
	quiet := fold(eventwire.Event{Kind: "turn_done", Receipt: &eventwire.CompletionReceipt{Verdict: "complete"}})
	loud := fold(eventwire.Event{Kind: "turn_done", Receipt: &eventwire.CompletionReceipt{Verdict: "partial", SaysSomething: true}})
	if len(quiet.Items) != 0 || len(loud.Items) != 1 || loud.Items[0].Kind != ItemReceipt {
		t.Fatalf("receipts: quiet=%+v loud=%+v", quiet.Items, loud.Items)
	}
}
