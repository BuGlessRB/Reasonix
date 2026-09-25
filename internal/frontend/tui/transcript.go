package tui

import (
	"slices"
	"strings"

	"reasonix/internal/contract/eventwire"
)

// ItemKind is what a transcript row is.
type ItemKind int

const (
	ItemUser ItemKind = iota
	ItemSay
	ItemTool
	ItemApproval
	ItemAsk
	ItemNotice
	ItemCompaction
	ItemReceipt
)

// Item is one row of the transcript. Which fields are set follows Kind.
type Item struct {
	ID   int
	Kind ItemKind

	// ItemUser. Pending is input the kernel has not taken into a turn yet;
	// Steer is input a running turn read at a tool boundary.
	Text    string
	Pending bool
	Steer   bool
	QueueID string

	// ItemSay.
	Reasoning string
	Done      bool
	ThoughtMs int64

	// ItemTool. Children are a sub-agent's calls, folded under its task.
	Tool     *eventwire.Tool
	Children []eventwire.Tool
	Running  bool

	// ItemApproval / ItemAsk. Verdict is how this screen settled it; empty
	// while it is still open.
	Approval *eventwire.Approval
	Ask      *eventwire.Ask
	Verdict  string

	// ItemNotice. Count folds a repeated notice into the row already there.
	Level string
	Code  string
	Count int

	Compaction *eventwire.Compaction
	Receipt    *eventwire.CompletionReceipt
}

// Terminal is how the last turn ended.
type Terminal int

const (
	TurnOpen Terminal = iota
	TurnCompleted
	TurnCancelled
	TurnFailed
	TurnIncomplete
)

// Transcript folds the event stream the way Studio's session reducer does, so
// a conversation reads the same in both. Nothing on the wire echoes what the
// user typed: the client adds its own row (AddUser).
type Transcript struct {
	Items    []Item
	Running  bool
	Terminal Terminal
	// EndReason is the kernel's account of a turn that did not complete.
	EndReason string
	Usage     *eventwire.Usage
	// TodosMoved says the kernel's task list changed and should be re-read.
	TodosMoved bool
	// QueueMoved says the durable input queue changed and should be re-read.
	QueueMoved bool
	next       int
}

func (t *Transcript) id() int {
	t.next++
	return t.next
}

// AddUser records what this screen sent. pending marks input handed to a
// running turn, which stays pending until the steer event says it was read.
func (t *Transcript) AddUser(text string, pending bool, queueID string) {
	t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemUser, Text: text, Pending: pending, QueueID: queueID})
}

// Decide seals an approval or ask this screen answered.
func (t *Transcript) Decide(id int, verdict string) {
	for i := range t.Items {
		if t.Items[i].ID == id {
			t.Items[i].Verdict = verdict
			return
		}
	}
}

// OpenPrompt is the approval or ask still waiting on an answer, if any.
func (t *Transcript) OpenPrompt() *Item {
	for i, it := range slices.Backward(t.Items) {
		if (it.Kind == ItemApproval || it.Kind == ItemAsk) && it.Verdict == "" {
			return &t.Items[i]
		}
	}
	return nil
}

// Apply folds one frame.
func (t *Transcript) Apply(ev eventwire.Event) {
	// A decision receipt is state synchronisation, not conversation: the card it
	// names settles, and the audit wording is not repeated as a row.
	if ev.DecisionReceipt != nil {
		t.sealByReceipt(ev.DecisionReceipt)
	}
	if ev.Kind == "notice" && ev.Code == "decision_receipt" {
		return
	}
	switch ev.Kind {
	case "turn_started":
		t.Running, t.Terminal, t.EndReason = true, TurnOpen, ""
	case "reasoning":
		t.appendSay(ev.Text, true)
	case "text":
		t.appendSay(ev.Text, false)
	case "message":
		t.foldMessage(ev)
	case "tool_dispatch", "tool_progress":
		if ev.Tool != nil {
			t.foldTool(*ev.Tool, true)
		}
	case "tool_result":
		if ev.Tool != nil {
			t.foldTool(*ev.Tool, false)
		}
	case "approval_request":
		if ev.Approval != nil {
			t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemApproval, Approval: ev.Approval})
		}
	case "ask_request":
		if ev.Ask != nil {
			t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemAsk, Ask: ev.Ask})
		}
	case "compaction_started":
		t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemCompaction, Compaction: ev.Compaction})
	case "compaction_done":
		for i, it := range slices.Backward(t.Items) {
			if it.Kind == ItemCompaction {
				t.Items[i].Compaction, t.Items[i].Done = ev.Compaction, true
				break
			}
		}
	case "steer":
		t.foldSteer(ev)
	case "todo_progress":
		t.TodosMoved = true
	case "inbox_changed":
		t.QueueMoved = true
	case "usage":
		t.Usage = ev.Usage
	case "notice":
		t.foldNotice(ev)
	case "turn_done":
		t.Running = false
		t.Terminal, t.EndReason = terminalOf(ev)
		t.sealSays()
		t.sealTools(ev.Err != "")
		if ev.Receipt != nil && ev.Receipt.SaysSomething {
			t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemReceipt, Receipt: ev.Receipt})
		}
	}
}

func terminalOf(ev eventwire.Event) (Terminal, string) {
	switch {
	case ev.Cancelled:
		return TurnCancelled, ev.Err
	case ev.Err != "":
		return TurnFailed, ev.Err
	case ev.Outcome != "":
		return TurnIncomplete, ev.Outcome
	}
	return TurnCompleted, ""
}

func (t *Transcript) openSay() int {
	if n := len(t.Items); n > 0 && t.Items[n-1].Kind == ItemSay && !t.Items[n-1].Done {
		return n - 1
	}
	return -1
}

func (t *Transcript) appendSay(text string, reasoning bool) {
	at := t.openSay()
	if at < 0 {
		t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemSay})
		at = len(t.Items) - 1
	}
	if reasoning {
		t.Items[at].Reasoning += text
	} else {
		t.Items[at].Text += text
	}
}

// foldMessage settles the open answer with the frame's own text, which wins
// over the deltas: it is what the transcript keeps, so a stream that dropped a
// chunk is repaired here rather than preserved. With no open answer (a rebuild,
// or a turn whose deltas never came) the frame is the answer.
func (t *Transcript) foldMessage(ev eventwire.Event) {
	at := -1
	for i, it := range slices.Backward(t.Items) {
		if it.Kind == ItemSay && !it.Done {
			at = i
			break
		}
	}
	if at < 0 {
		if ev.Text == "" && ev.Reasoning == "" {
			return
		}
		t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemSay, Text: ev.Text, Reasoning: ev.Reasoning, Done: true, ThoughtMs: ev.ThoughtMs})
		return
	}
	it := &t.Items[at]
	it.Done = true
	if ev.Text != "" {
		it.Text = ev.Text
	}
	if ev.Reasoning != "" {
		it.Reasoning = ev.Reasoning
	}
	if ev.ThoughtMs > 0 {
		it.ThoughtMs = ev.ThoughtMs
	}
}

// mergeTool lays a later frame of the same call over the earlier one. The wire
// omits empty fields, so a result does not erase the arguments its dispatch
// carried, and partial is set only by the frame that says so.
func mergeTool(prev, next eventwire.Tool) eventwire.Tool {
	out := prev
	if next.Name != "" {
		out.Name = next.Name
	}
	if next.Args != "" {
		out.Args = next.Args
	}
	if next.Output != "" {
		out.Output = next.Output
	}
	if next.Err != "" {
		out.Err = next.Err
	}
	if next.RefusalCode != "" {
		out.RefusalCode = next.RefusalCode
	}
	if next.Diff != "" {
		out.Diff, out.Added, out.Removed = next.Diff, next.Added, next.Removed
	}
	if next.DurationMs > 0 {
		out.DurationMs = next.DurationMs
	}
	if next.Execution != nil {
		out.Execution = next.Execution
	}
	if next.Bound != nil {
		out.Bound = next.Bound
	}
	out.Partial = next.Partial
	return out
}

func (t *Transcript) foldTool(tool eventwire.Tool, running bool) {
	if tool.ParentID != "" {
		for i := range t.Items {
			it := &t.Items[i]
			if it.Kind != ItemTool || it.Tool.ID != tool.ParentID {
				continue
			}
			for k := range it.Children {
				if it.Children[k].ID == tool.ID {
					it.Children[k] = mergeTool(it.Children[k], tool)
					return
				}
			}
			it.Children = append(it.Children, tool)
			return
		}
	}
	if tool.ID != "" {
		for i := range t.Items {
			if it := &t.Items[i]; it.Kind == ItemTool && it.Tool.ID == tool.ID {
				merged := mergeTool(*it.Tool, tool)
				it.Tool, it.Running = &merged, running
				return
			}
		}
	}
	// A dispatch closes the answer before it: the model moved on to a call.
	if at := t.openSay(); at >= 0 {
		t.Items[at].Done = true
	}
	t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemTool, Tool: &tool, Running: running})
}

// foldSteer moves pending input to where the turn read it: the work that ran
// while it waited was not done about it.
func (t *Transcript) foldSteer(ev eventwire.Event) {
	at := -1
	for i, it := range t.Items {
		if it.Kind != ItemUser || !it.Pending {
			continue
		}
		if (ev.ItemID != "" && it.QueueID == ev.ItemID) || (ev.ItemID == "" && it.Text == ev.Text) {
			at = i
			break
		}
	}
	if at < 0 {
		if strings.TrimSpace(ev.Text) != "" && !ev.HostAuthored {
			t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemUser, Text: ev.Text, Steer: true})
		}
		return
	}
	row := t.Items[at]
	row.Pending, row.Steer = false, true
	t.Items = append(append(t.Items[:at:at], t.Items[at+1:]...), row)
}

// foldNotice keeps notices about the machine out of the conversation unless
// they need attention now, and folds a repeat into the row it repeats.
func (t *Transcript) foldNotice(ev eventwire.Event) {
	level := ev.Level
	if level == "" {
		level = "info"
	}
	if ev.Audience == "operator" && level == "info" {
		return
	}
	if n := len(t.Items); n > 0 {
		last := &t.Items[n-1]
		same := last.Kind == ItemNotice && last.Level == level &&
			((last.Code != "" && last.Code == ev.Code) || (last.Code == "" && ev.Code == "" && last.Text == ev.Text))
		if same {
			last.Count = max(last.Count, 1) + 1
			return
		}
	}
	t.Items = append(t.Items, Item{ID: t.id(), Kind: ItemNotice, Level: level, Code: ev.Code, Text: ev.Text})
}

func (t *Transcript) sealSays() {
	for i := range t.Items {
		if t.Items[i].Kind == ItemSay {
			t.Items[i].Done = true
		}
	}
}

// sealTools stops every call still drawn as running: a turn that ended owns no
// call in flight, and a failed turn's open call did not finish.
func (t *Transcript) sealTools(failed bool) {
	for i := range t.Items {
		it := &t.Items[i]
		if it.Kind == ItemTool && it.Running {
			it.Running = false
			if failed && it.Tool.Err == "" && it.Tool.Output == "" {
				it.Tool.Err = "interrupted"
			}
		}
	}
}

// sealByReceipt settles a prompt answered somewhere else: the kernel's receipt
// names it, and this screen must stop offering an answer to it.
func (t *Transcript) sealByReceipt(r *eventwire.DecisionReceipt) {
	for i := range t.Items {
		it := &t.Items[i]
		if it.Verdict != "" {
			continue
		}
		if (it.Kind == ItemApproval && it.Approval.ID == r.ID) || (it.Kind == ItemAsk && it.Ask.ID == r.ID) {
			it.Verdict = "elsewhere"
		}
	}
}
