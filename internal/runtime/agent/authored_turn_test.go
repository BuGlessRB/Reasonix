package agent

import (
	"context"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
)

type turnStartProvider struct{}

func (p *turnStartProvider) Name() string { return "turn-start" }

func (p *turnStartProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "answered"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// runTurns runs each input as its own turn and returns the TurnStarted events,
// in order. The agent is built with no host around it at all: this layer cannot
// import control, so nothing a checkpoint knows can reach these numbers.
func runTurns(t *testing.T, session *Session, inputs ...string) []event.Event {
	t.Helper()
	var started []event.Event
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			started = append(started, e)
		}
	})
	a := New(&turnStartProvider{}, tool.NewRegistry(), session, Options{}, sink)
	for _, in := range inputs {
		if err := a.Run(context.Background(), in); err != nil {
			t.Fatalf("Run(%q): %v", in, err)
		}
	}
	return started
}

func requireNamed(t *testing.T, e event.Event, wantTurn int) int {
	t.Helper()
	if e.AuthoredTurn == nil || e.MsgIndex == nil {
		t.Fatalf("turn_started named no message: authored=%v index=%v", e.AuthoredTurn, e.MsgIndex)
	}
	if *e.AuthoredTurn != wantTurn {
		t.Fatalf("authored turn = %d, want %d", *e.AuthoredTurn, wantTurn)
	}
	return *e.MsgIndex
}

func TestTurnStartedNamesTheAuthoredMessageItIsAbout(t *testing.T) {
	session := NewSession("system")
	started := runTurns(t, session, "first", "second")
	if len(started) != 2 {
		t.Fatalf("turn_started events = %d, want 2", len(started))
	}
	// The system message holds index 0, so the first authored turn is 1 and the
	// message it opens is 1 — two numbers that are equal once and never again.
	if index := requireNamed(t, started[0], 1); index != 1 {
		t.Fatalf("first turn message index = %d, want 1", index)
	}
	if index := requireNamed(t, started[1], 2); index <= *started[0].MsgIndex {
		t.Fatalf("second turn message index = %d, want past the first turn's", index)
	}
}

func TestLiveTurnStartAndDisplayIndexNameTheSameMessage(t *testing.T) {
	session := NewSession("system")
	started := runTurns(t, session, "first", "second")
	msgs := session.Snapshot()
	digest, err := digestSessionMessages(msgs)
	if err != nil {
		t.Fatalf("digestSessionMessages: %v", err)
	}
	idx := BuildSessionDisplayIndex(msgs, 1, true, digest)
	if idx == nil {
		t.Fatal("BuildSessionDisplayIndex returned nil")
	}
	for turn, e := range started {
		index := requireNamed(t, e, turn+1)
		if index < 0 || index >= len(idx.Entries) {
			t.Fatalf("turn %d named index %d, outside a transcript of %d", turn+1, index, len(idx.Entries))
		}
		entry := idx.Entries[index]
		if entry.Role != provider.RoleUser || !entry.StartsTurn || entry.AuthoredTurn != *e.AuthoredTurn {
			t.Fatalf("live turn %d named index %d; the record there is %+v", *e.AuthoredTurn, index, entry)
		}
	}
}

func TestMidTurnSteerMintsNoAuthoredTurn(t *testing.T) {
	session := NewSession("system")
	runTurns(t, session, "first")
	session.Add(provider.Message{Role: provider.RoleUser, Content: midTurnSteerMessage("keep going", false)})
	started := runTurns(t, session, "second")
	if len(started) != 1 {
		t.Fatalf("turn_started events = %d, want 1", len(started))
	}
	requireNamed(t, started[0], 2)
}

func TestSyntheticUserTurnMintsNoAuthoredTurn(t *testing.T) {
	session := NewSession("system")
	runTurns(t, session, "first")
	injected := runTurns(t, session, "Continue pursuing the active goal: finish the migration.")
	if len(injected) != 1 {
		t.Fatalf("turn_started events = %d, want 1", len(injected))
	}
	if injected[0].AuthoredTurn != nil || injected[0].MsgIndex != nil {
		t.Fatalf("host-injected turn named a message: authored=%v index=%v",
			*injected[0].AuthoredTurn, *injected[0].MsgIndex)
	}
	// The number it did not take is still there for the next real turn.
	next := runTurns(t, session, "second")
	requireNamed(t, next[0], 2)
}

// TestPlannerNeverNamesTheTurnAtTheParentSink keeps the planner's own turn
// identity out of the conversation's: it runs over its own session, so a start
// it announces names a message in that transcript. Under a host boundary it
// announces nothing; standalone it still does, which is why the coordinator's
// sink filter is not dead code.
func TestPlannerNeverNamesTheTurnAtTheParentSink(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		name := "standalone"
		if hosted {
			name = "host owns the boundary"
		}
		t.Run(name, func(t *testing.T) {
			var starts []event.Event
			sink := event.FuncSink(func(e event.Event) {
				if e.Kind == event.TurnStarted {
					starts = append(starts, e)
				}
			})
			// The planner's transcript is longer, so a name minted over it
			// cannot be mistaken for one minted over the executor's.
			plannerSession := NewSession("planner system")
			plannerSession.Add(provider.Message{Role: provider.RoleUser, Content: "earlier planner turn"})
			plannerSession.Add(provider.Message{Role: provider.RoleAssistant, Content: "earlier plan"})
			exec := New(&turnStartProvider{}, tool.NewRegistry(), NewSession("system"), Options{}, sink)
			c := NewCoordinatorWithPlannerPolicy(
				&turnStartProvider{}, plannerSession, nil, tool.NewRegistry(), Options{}, exec, 0, sink,
				func(context.Context, string) PlannerDecision {
					return PlannerDecision{Route: PlannerRoutePlanAndExecute, Depth: PlannerDepthFull, Reason: "test"}
				},
			)
			ctx := context.Background()
			if hosted {
				ctx = WithHostTurnBoundary(ctx, HostTurnBoundary{})
			}
			if err := c.Run(ctx, "第一句用户输入"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			named := 0
			for _, e := range starts {
				if e.AuthoredTurn == nil {
					continue
				}
				named++
				if *e.MsgIndex != 1 {
					t.Fatalf("a start named message %d; the executor's turn is message 1, the planner's is not this turn",
						*e.MsgIndex)
				}
			}
			want := 1
			if hosted {
				want = 0
			}
			if named != want {
				t.Fatalf("named starts at the parent sink = %d, want %d", named, want)
			}
		})
	}
}

// TestLandingHoldsTheMessageToTheAnnouncedIdentity checks the seam holding a
// published name to the message that lands. The mismatch is constructed —
// control measures the index one statement before handing over the identity —
// so this proves the check fires, not that anything reaches it.
func TestLandingHoldsTheMessageToTheAnnouncedIdentity(t *testing.T) {
	land := func(id AuthoredTurnIdentity, text string) []event.Event {
		var notices []event.Event
		sink := event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice && e.Code == event.NoticeCodeTurnIdentityMismatch {
				notices = append(notices, e)
			}
		})
		session := NewSession("system")
		a := New(&turnStartProvider{}, tool.NewRegistry(), session, Options{}, sink)
		ctx := WithHostTurnBoundary(context.Background(), HostTurnBoundary{Authored: &id})
		a.LandAuthoredUserMessage(ctx, provider.Message{Role: provider.RoleUser, Content: text})
		if got := session.Snapshot()[1].RawContent; got != id.Raw {
			t.Fatalf("landed RawContent = %q, want the announced identity's %q", got, id.Raw)
		}
		return notices
	}

	if notices := land(AuthoredTurnIdentity{AuthoredTurn: 1, MsgIndex: 1, Raw: "第一句"}, "第一句"); len(notices) != 0 {
		t.Fatalf("a message that landed where it was named reported %d mismatches", len(notices))
	}
	notices := land(AuthoredTurnIdentity{AuthoredTurn: 1, MsgIndex: 7, Raw: "第一句"}, "第一句")
	if len(notices) != 1 {
		t.Fatalf("a message that landed elsewhere reported %d mismatches, want 1", len(notices))
	}
	if notices[0].Audience != event.NoticeAudienceOperator {
		t.Fatalf("mismatch audience = %q, want operator: this is about the machine, not the conversation", notices[0].Audience)
	}
}
