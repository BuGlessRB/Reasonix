package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// hostSpeech collects every host observation the run appended to a tool result.
func hostSpeech(s *Session) []string {
	var said []string
	for _, m := range s.Messages {
		if m.Role == provider.RoleTool && strings.Contains(m.Content, "[host]") {
			said = append(said, m.Content)
		}
	}
	return said
}

// A turn whose very first round fails must not be stopped for it. The account
// only settles rounds that produced something, so a failing opening round is
// the storm breaker's business and the runway does not touch it.
func TestOpeningFailureNeitherSpeaksNorPauses(t *testing.T) {
	prov := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "x1", Name: "missing_tool", Arguments: `{}`}}},
		testutil.Turn{Text: "That tool does not exist; here is what I can do instead."},
	)
	session := NewSession("")
	a := New(prov, tool.NewRegistry(), session, Options{}, event.Discard)
	ctx := WithDeliveryExecutionScope(context.Background(), DeliveryExecutionScope{ID: "goal-1", TaskText: "finish"})
	if err := a.Run(ctx, "work"); err != nil {
		t.Fatalf("a first-round failure must not pause the goal: %v", err)
	}
	if said := hostSpeech(session); len(said) > 0 {
		t.Fatalf("host spoke on the opening round: %q", said)
	}
}

// The same shape one round later: fail, recover, carry on. Neither the failure
// nor the recovery may cost the turn its opening runway.
func TestFailureThenRecoveryLeavesTheAccountUntouched(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	prov := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "x1", Name: "missing_tool", Arguments: `{}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "r1", Name: "read_file", Arguments: `{"path":"a"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "x2", Name: "missing_tool", Arguments: `{"other":1}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "r2", Name: "read_file", Arguments: `{"path":"b"}`}}},
		testutil.Turn{Text: "Found it."},
	)
	session := NewSession("")
	a := New(prov, reg, session, Options{}, event.Discard)
	ctx := WithDeliveryExecutionScope(context.Background(), DeliveryExecutionScope{ID: "goal-1", TaskText: "finish"})
	if err := a.Run(ctx, "work"); err != nil {
		t.Fatalf("a recovering turn must not pause: %v", err)
	}
	if said := hostSpeech(session); len(said) > 0 {
		t.Fatalf("host spoke on a recovering turn: %q", said)
	}
}

// Short turns are the common case and must never hear from the host: the
// opening balance covers several dead rounds before there is anything to say.
func TestShortTurnsNeverHearFromTheHost(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	for rounds := 1; rounds <= 4; rounds++ {
		turns := make([]testutil.Turn, 0, rounds+1)
		for i := range rounds {
			turns = append(turns, testutil.Turn{ToolCalls: []provider.ToolCall{{
				ID: fmt.Sprintf("r%d", i), Name: "read_file", Arguments: `{"path":"same"}`,
			}}})
		}
		turns = append(turns, testutil.Turn{Text: "Done."})
		session := NewSession("")
		a := New(testutil.NewMock("m", turns...), reg, session, Options{}, event.Discard)
		if err := a.Run(context.Background(), "work"); err != nil {
			t.Fatalf("%d-round turn: %v", rounds, err)
		}
		if said := hostSpeech(session); len(said) > 0 {
			t.Errorf("host spoke on a %d-round turn re-reading one path: %q", rounds, said)
		}
	}
}

// Every round failing identically stays the storm breaker's pause, not the
// runway's — the two guards must not double-report the same turn.
func TestAllFailingRoundsPauseOnTheStormBreaker(t *testing.T) {
	turns := make([]testutil.Turn, 0, stormBreakThreshold+1)
	for i := range stormBreakThreshold {
		turns = append(turns, testutil.Turn{ToolCalls: []provider.ToolCall{{
			ID: fmt.Sprintf("x%d", i), Name: "missing_tool", Arguments: `{}`,
		}}})
	}
	turns = append(turns, testutil.Turn{Text: "The same host failure repeated."})
	a := New(testutil.NewMock("m", turns...), tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	ctx := WithDeliveryExecutionScope(context.Background(), DeliveryExecutionScope{ID: "goal-1", TaskText: "finish"})
	info, ok := InspectRunPause(a.Run(ctx, "work"))
	if !ok || info.Key != "goal repeated host outcome" {
		t.Fatalf("pause = %+v ok=%v, want the storm breaker to own an all-failing turn", info, ok)
	}
}
