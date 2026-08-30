package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/testenv"
	"reasonix/internal/tool"
)

// Capacity and economics answer different questions: how close the prompt is to
// not fitting, and how much it costs to replay. A declared 1M window makes a
// 300K prompt legal without making it economical, so the boundary that decides
// maintenance is whichever one is reached first.
func TestMaintenanceBoundaryIsTheNearerOfCapacityAndEconomics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		window   int
		ratio    float64
		soft     int
		trigger  int
		boundary string
	}{
		{"million-window folds on economics", 1_000_000, 0.85, 0, defaultContextSoftLimitTokens, "economic"},
		{"small window keeps capacity first", 128_000, 0.85, 0, 108_800, "capacity"},
		{"configured soft limit wins when lower", 1_000_000, 0.85, 96_000, 96_000, "economic"},
		{"configured soft limit loses when higher", 200_000, 0.85, 400_000, 170_000, "capacity"},
		{"negative soft limit disables economics", 1_000_000, 0.85, -1, 850_000, "capacity"},
		{"no declared window leaves both unset", 0, 0.85, 0, 0, ""},
		// A ratio past the window is how a user turns automatic maintenance off.
		{"ratio past the window stays off", 1_000_000, 2, 0, 2_000_000, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := New(nil, tool.NewRegistry(), &Session{}, Options{
				ContextWindow:     tc.window,
				CompactRatio:      tc.ratio,
				CompactionBudgets: CompactionBudgets{ContextSoftLimitTokens: tc.soft},
			}, event.Discard)
			if got := a.compactTrigger(); got != tc.trigger {
				t.Errorf("compactTrigger = %d, want %d", got, tc.trigger)
			}
			if got := a.compactBoundary(); got != tc.boundary {
				t.Errorf("compactBoundary = %q, want %q", got, tc.boundary)
			}
		})
	}
}

// economicFixture is a session whose visible input sits far above the economic
// boundary and far below the capacity one, which is exactly the shape that ran
// for 2150 rounds at 321K tokens inside a declared 1M window.
const economicActiveTurnAt int64 = 1_700_000_000_000

func economicFixture(t *testing.T, soft int, activeTurnRound int) (*Agent, *recordSink) {
	t.Helper()
	sess := &Session{Messages: []provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: "long-running task"},
	}}
	body := strings.Repeat("x", 24*1024)
	for i := range 40 {
		turn := provider.Message{Role: provider.RoleUser, Content: body}
		if i == activeTurnRound {
			turn.CreatedAt = economicActiveTurnAt
		}
		sess.Messages = append(sess.Messages,
			provider.Message{Role: provider.RoleAssistant, Content: "step " + string(rune('a'+i%26))},
			turn)
	}
	sink := &recordSink{}
	a := New(&fakeProvider{reply: "digest"}, tool.NewRegistry(), sess, Options{
		ContextWindow:     1_000_000,
		CompactRatio:      0.85,
		RecentKeep:        2,
		ArchiveDir:        testenv.TempDir(t),
		CompactionBudgets: CompactionBudgets{ContextSoftLimitTokens: soft},
	}, sink)
	return a, sink
}

// A prompt above the economic boundary must be maintained even though it is
// nowhere near the window share, and one below it must not be.
func TestEconomicTriggerFoldsBelowCapacityShare(t *testing.T) {
	a, sink := economicFixture(t, 0, -1)
	visible := a.estimatedVisibleRequestTokens(a.modelVisibleMessages())
	if visible <= a.economicCompactTrigger() || visible >= a.capacityCompactTrigger() {
		t.Fatalf("fixture input %d is not between the economic (%d) and capacity (%d) boundaries",
			visible, a.economicCompactTrigger(), a.capacityCompactTrigger())
	}
	if _, err := a.contextManager().Prepare(context.Background(), ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
		t.Fatal(err)
	}
	if got := a.currentProjectionVersion(); got == 0 {
		t.Fatal("input above the economic boundary installed no projection")
	}
	if got := appliedMaintenanceEvents(sink); got != 1 {
		t.Fatalf("applied maintenance events = %d, want 1", got)
	}
}

func TestInputBelowBothBoundariesIsNotMaintained(t *testing.T) {
	a, sink := economicFixture(t, 4_000_000, -1)
	if _, err := a.contextManager().Prepare(context.Background(), ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
		t.Fatal(err)
	}
	if got := a.currentProjectionVersion(); got != 0 {
		t.Fatalf("input below both boundaries installed projection version %d", got)
	}
	if got := appliedMaintenanceEvents(sink); got != 0 {
		t.Fatalf("applied maintenance events = %d, want 0", got)
	}
}

// growTurn appends rounds to the running turn, which is what a tool loop does
// between two Prepare calls and what keeps its history unfoldable.
func growTurn(a *Agent, rounds int) {
	body := strings.Repeat("x", 24*1024)
	for i := range rounds {
		a.sess.conversation.Add(provider.Message{Role: provider.RoleAssistant, Content: "more " + string(rune('a'+i%26))})
		a.sess.conversation.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "t", Name: "read_file", Content: body})
	}
}

func maintenanceCodes(sink *recordSink, status string) []string {
	var codes []string
	for _, got := range sink.kinds(event.ContextMaintenanceEvent) {
		if got.Maintenance != nil && got.Maintenance.Status == status {
			codes = append(codes, got.Maintenance.Code)
		}
	}
	return codes
}

// A turn folds at most once, and the rounds after it keep arriving above the
// boundary. Without a verdict for those rounds the trajectory shows a session
// that never tried to maintain its context, which is what this whole path was
// missing: the attempt happened and freed nothing.
func TestSecondMaintenanceInOneTurnReportsAlreadyCompacted(t *testing.T) {
	// The running turn starts late enough that the rounds before it fold, so
	// the first attempt succeeds and the turn is marked as already compacted.
	a, sink := economicFixture(t, 0, 30)
	a.activeTurnCreatedAt.Store(economicActiveTurnAt)
	ctx := context.Background()
	if _, err := a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
		t.Fatal(err)
	}
	if a.currentProjectionVersion() == 0 {
		t.Fatal("first attempt installed no projection; the fixture cannot show the second")
	}
	// The turn keeps running. Its own rounds are held verbatim, so the visible
	// input climbs back over the boundary with nothing new left to fold.
	growTurn(a, 30)
	if _, err := a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
		t.Fatal(err)
	}
	codes := maintenanceCodes(sink, "noop")
	if len(codes) != 1 || codes[0] != string(NoopAlreadyCompactedThisTurn) {
		t.Fatalf("noop codes = %v, want exactly [%s]", codes, NoopAlreadyCompactedThisTurn)
	}
}

// The threshold stays crossed for the rest of the turn, so the same verdict is
// reached on every later round. Reporting it once is what keeps a trajectory
// readable; reporting it never is what made this invisible.
func TestRepeatedNoopInOneTurnReportsOnce(t *testing.T) {
	a, sink := economicFixture(t, 0, 30)
	a.activeTurnCreatedAt.Store(economicActiveTurnAt)
	ctx := context.Background()
	if _, err := a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		growTurn(a, 10)
		if _, err := a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
			t.Fatal(err)
		}
	}
	if codes := maintenanceCodes(sink, "noop"); len(codes) != 1 {
		t.Fatalf("noop codes = %v, want exactly one report for the turn", codes)
	}
}

// A turn that is the whole transcript has nothing behind it to fold. That is a
// different verdict from an exhausted history, and the two are told apart by
// the code rather than by reading the sentence beside it.
func TestActiveTurnBoundaryLeavesNothingToFold(t *testing.T) {
	body := strings.Repeat("x", 24*1024)
	// Too large to be pinned as a brief, so the fixed prefix is the system
	// message alone and the running turn starts exactly where the fold would.
	sess := &Session{Messages: []provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: body, CreatedAt: economicActiveTurnAt},
	}}
	for i := range 40 {
		sess.Messages = append(sess.Messages,
			provider.Message{Role: provider.RoleAssistant, Content: "step " + string(rune('a'+i%26))},
			provider.Message{Role: provider.RoleTool, ToolCallID: "t", Name: "read_file", Content: body})
	}
	sink := &recordSink{}
	a := New(&fakeProvider{reply: "digest"}, tool.NewRegistry(), sess, Options{
		ContextWindow: 1_000_000, CompactRatio: 0.85, RecentKeep: 2, ArchiveDir: testenv.TempDir(t),
	}, sink)
	a.activeTurnCreatedAt.Store(economicActiveTurnAt)
	if _, err := a.contextManager().Prepare(context.Background(), ContextPreparePolicy{Trigger: CompactionTriggerPressure}); err != nil {
		t.Fatal(err)
	}
	codes := maintenanceCodes(sink, "noop")
	if len(codes) != 1 || codes[0] != string(NoopActiveTurnBoundary) {
		t.Fatalf("noop codes = %v, want exactly [%s]", codes, NoopActiveTurnBoundary)
	}
}
