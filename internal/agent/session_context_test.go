package agent

import (
	"context"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
	"reasonix/internal/tool"
)

func TestRunInjectsSessionContextBeforeRealUserAndDeduplicatesDigest(t *testing.T) {
	first := sessioncontext.Build(sessioncontext.Sections{
		Environment: "go/linux",
		Workspace:   `Current workspace: "/work"`,
	})
	second := sessioncontext.Build(sessioncontext.Sections{
		Environment:      "go/linux",
		Workspace:        `Current workspace: "/work"`,
		BackgroundMemory: "new fact index",
	})
	prov := testutil.NewMock("m",
		testutil.Turn{Text: "one"}, testutil.Turn{Text: "two"}, testutil.Turn{Text: "three"})
	sess := NewSession("stable system")
	a := New(prov, tool.NewRegistry(), sess, Options{}, event.Discard)
	ctx := WithTurnContextBundle(context.Background(), TurnContextBundle{Executor: first})

	if err := a.Run(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	msgs := sess.Snapshot()
	if len(msgs) != 4 || msgs[1].Origin != provider.MessageOriginHost || msgs[1].Content != first.Content ||
		!IsUserAuthoredTurnMessage(msgs[2]) || msgs[2].RawContent != "hello" || msgs[2].CreatedAt == 0 {
		t.Fatalf("first turn history = %+v, want system/context/user/assistant", msgs)
	}
	if msgs[1].RawContent != "" || msgs[1].CreatedAt != 0 || IsUserAuthoredTurnMessage(msgs[1]) {
		t.Fatalf("context leaked user-turn metadata: %+v", msgs[1])
	}
	if preview, turns := SessionPreviewFromMessages(msgs); preview != "hello" || turns != 1 || strings.Contains(preview, "session-context") {
		t.Fatalf("context leaked into preview/turn count: preview=%q turns=%d", preview, turns)
	}
	if err := a.Run(ctx, "again"); err != nil {
		t.Fatal(err)
	}
	if got := countSessionContexts(sess.Snapshot()); got != 1 {
		t.Fatalf("same digest produced %d context messages, want 1", got)
	}
	ctx = WithTurnContextBundle(context.Background(), TurnContextBundle{Executor: second})
	if err := a.Run(ctx, "after memory change"); err != nil {
		t.Fatal(err)
	}
	if got := countSessionContexts(sess.Snapshot()); got != 2 {
		t.Fatalf("replacement digest produced %d context messages, want 2", got)
	}
	lastRequest := prov.LastRequest()
	if lastRequest == nil {
		t.Fatal("provider received no request")
	}
	for _, message := range lastRequest.Messages {
		if message.Origin != "" || message.RawContent != "" || message.CreatedAt != 0 {
			t.Fatalf("provider request leaked host provenance/display metadata: %+v", message)
		}
	}
}

func TestSessionContextPlannerSelectionAndSyntheticBootstrap(t *testing.T) {
	executor := sessioncontext.Build(sessioncontext.Sections{SkillsCatalog: "executor skill"})
	planner := sessioncontext.Build(sessioncontext.Sections{SkillsCatalog: "read-only skill"})
	bundle := TurnContextBundle{Executor: executor, Planner: planner}
	sess := NewSession("system")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "legacy request"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "legacy answer"})
	a := New(nil, tool.NewRegistry(), sess, Options{}, event.Discard)

	ctx := withPlannerTurnContext(WithTurnContextBundle(context.Background(), TurnContextBundle{
		Executor: executor, Planner: planner, BootstrapOnly: true,
	}))
	if !a.AppendTurnContext(ctx) {
		t.Fatal("legacy synthetic continuation should bootstrap one v1 snapshot")
	}
	if got := sess.Messages[len(sess.Messages)-1].Content; got != planner.Content {
		t.Fatalf("planner received %q, want read-only snapshot", got)
	}
	ctx = withPlannerTurnContext(WithTurnContextBundle(context.Background(), TurnContextBundle{
		Executor: executor, Planner: sessioncontext.Build(sessioncontext.Sections{SkillsCatalog: "changed"}), BootstrapOnly: true,
	}))
	if a.AppendTurnContext(ctx) {
		t.Fatal("synthetic continuation consumed a pending catalog change")
	}
	if got := countSessionContexts(sess.Snapshot()); got != 1 {
		t.Fatalf("bootstrap context count = %d, want 1", got)
	}

	ordinary := withPlannerTurnContext(WithTurnContextBundle(context.Background(), bundle))
	if a.AppendTurnContext(ordinary) {
		t.Fatal("same planner digest should deduplicate")
	}
}

func TestSessionContextDiagnosticsAreContentFreeAndAttributeChanges(t *testing.T) {
	prov := &cacheDiagProvider{chunks: [][]provider.Chunk{
		{{Type: provider.ChunkText, Text: "one"}, {Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 20, CacheMissTokens: 20}}},
		{{Type: provider.ChunkText, Text: "two"}, {Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 20, CacheHitTokens: 10, CacheMissTokens: 10}}},
	}}
	var diagnostics []*event.CacheDiagnostics
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.Usage {
			diagnostics = append(diagnostics, e.CacheDiagnostics)
		}
	})
	a := New(prov, tool.NewRegistry(), NewSession("stable"), Options{}, sink)
	first := sessioncontext.Build(sessioncontext.Sections{Workspace: "/secret/path", BackgroundMemory: "secret fact"})
	second := sessioncontext.Build(sessioncontext.Sections{Workspace: "/secret/path", BackgroundMemory: "changed secret fact"})
	for i, snapshot := range []sessioncontext.Snapshot{first, second} {
		ctx := WithTurnContextBundle(context.Background(), TurnContextBundle{Executor: snapshot})
		if err := a.Run(ctx, []string{"first", "second"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	if len(diagnostics) != 2 || diagnostics[0] == nil || diagnostics[1] == nil {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	firstDiag, secondDiag := diagnostics[0].SessionContext, diagnostics[1].SessionContext
	if firstDiag == nil || secondDiag == nil || firstDiag.Digest != first.Digest || secondDiag.Digest != second.Digest {
		t.Fatalf("session diagnostics = first %+v second %+v", firstDiag, secondDiag)
	}
	if strings.Join(firstDiag.Reasons, ",") != "first_seen" || strings.Join(secondDiag.Reasons, ",") != "memory_changed" {
		t.Fatalf("context reasons = %v then %v", firstDiag.Reasons, secondDiag.Reasons)
	}
	if !secondDiagHasPrefixReason(diagnostics[1], "session_context") || secondDiag.TargetRole != "executor" {
		t.Fatalf("second diagnostics = %+v", diagnostics[1])
	}
	encoded := firstDiag.Digest + firstDiag.Workspace.Digest + firstDiag.BackgroundMemory.Digest
	if strings.Contains(encoded, "secret") || firstDiag.Workspace.Chars != len("/secret/path") {
		t.Fatalf("diagnostics leaked content or wrong count: %+v", firstDiag)
	}
}

func TestToollessPlannerPublishesRoleSpecificContextDiagnostics(t *testing.T) {
	plannerProvider := testutil.NewMock("planner",
		testutil.Turn{Text: "first plan", Usage: &provider.Usage{PromptTokens: 10, CacheMissTokens: 10}},
		testutil.Turn{Text: "second plan", Usage: &provider.Usage{PromptTokens: 12, CacheHitTokens: 6, CacheMissTokens: 6}},
	)
	var diagnostics []*event.CacheDiagnostics
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.Usage && e.Source == event.UsageSourcePlanner {
			diagnostics = append(diagnostics, e.CacheDiagnostics)
		}
	})
	coordinator := NewCoordinator(plannerProvider, NewSession("planner system"), nil, nil, Options{}, nil, 0, sink, nil)
	executor := sessioncontext.Build(sessioncontext.Sections{SkillsCatalog: "main-only"})
	first := sessioncontext.Build(sessioncontext.Sections{SkillsCatalog: "read-only", BackgroundMemory: "one"})
	second := sessioncontext.Build(sessioncontext.Sections{SkillsCatalog: "read-only", BackgroundMemory: "two"})
	for i, snapshot := range []sessioncontext.Snapshot{first, second} {
		ctx := WithTurnContextBundle(context.Background(), TurnContextBundle{Executor: executor, Planner: snapshot})
		if _, err := coordinator.plan(ctx, []string{"first task", "second task"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	if len(diagnostics) != 2 || diagnostics[0] == nil || diagnostics[1] == nil {
		t.Fatalf("planner diagnostics = %+v", diagnostics)
	}
	if firstDiag := diagnostics[0].SessionContext; firstDiag == nil || firstDiag.TargetRole != "planner" ||
		firstDiag.Digest != first.Digest || strings.Join(firstDiag.Reasons, ",") != "first_seen" {
		t.Fatalf("first planner context diagnostics = %+v", firstDiag)
	}
	if secondDiag := diagnostics[1].SessionContext; secondDiag == nil || secondDiag.Digest != second.Digest ||
		strings.Join(secondDiag.Reasons, ",") != "memory_changed" || !secondDiagHasPrefixReason(diagnostics[1], "session_context") {
		t.Fatalf("second planner context diagnostics = %+v / %+v", secondDiag, diagnostics[1])
	}
	request := plannerProvider.LastRequest()
	if request == nil || len(request.Messages) < 3 || request.Messages[len(request.Messages)-2].Content != second.Content {
		t.Fatalf("planner request did not retain independent latest context: %+v", request)
	}
}

func countSessionContexts(messages []provider.Message) int {
	count := 0
	for _, message := range messages {
		if isSessionContextMessage(message) {
			count++
		}
	}
	return count
}

func secondDiagHasPrefixReason(diagnostics *event.CacheDiagnostics, want string) bool {
	return slices.Contains(diagnostics.PrefixChangeReasons, want)
}
