package agent

import (
	"context"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func toolHeavySession(turns int) *Session {
	sess := NewSession("sys")
	for i := range turns {
		sess.Add(provider.Message{Role: provider.RoleUser, Content: "turn " + strings.Repeat("x", 80) + string(rune('A'+i%26))})
		sess.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c", Name: "read", Arguments: "{}"}}})
		sess.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "c", Name: "read", Content: strings.Repeat("tool-output ", 200)})
	}
	return sess
}

func projectionHasSummary(a *Agent) bool {
	return slices.ContainsFunc(a.compactionState.Projection.Messages, isCompactionSummary)
}

// Tool-result maintenance rewrites the model-visible view, so it must start from
// the active projection. Rebuilding from canonical throws the summary away and
// inflates the context it was supposed to shrink.
func TestSnipPreservesSummaryProjection(t *testing.T) {
	sess := toolHeavySession(12)
	a := New(&fakeProvider{reply: "DIGEST body"}, tool.NewRegistry(), sess, Options{
		ContextWindow: 4000, RecentKeep: 2, ArchiveDir: t.TempDir(),
	}, event.Discard)

	if err := a.CompactNow(context.Background(), ""); err != nil {
		t.Fatalf("CompactNow: %v", err)
	}
	if !projectionHasSummary(a) {
		t.Fatal("setup: compaction produced no summary projection")
	}
	compacted := estimateMessagesTokens(a.compactionState.Projection.Messages)

	if err := a.snipToProjection(context.Background()); err != nil {
		t.Fatalf("snipToProjection: %v", err)
	}

	if !projectionHasSummary(a) {
		t.Error("snip discarded the summary projection")
	}
	if got := estimateMessagesTokens(a.compactionState.Projection.Messages); got > compacted {
		t.Errorf("snip grew the projection: %d -> %d tokens", compacted, got)
	}
}

// The same stale results must not be re-reported every turn: once snipped they
// carry the marker, so a second pass has nothing fresh to announce.
func TestSnipDoesNotRepeatItsNotice(t *testing.T) {
	var notices []string
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice && strings.Contains(e.Text, "stale tool results") {
			notices = append(notices, e.Text)
		}
	})
	a := New(&fakeProvider{reply: "unused"}, tool.NewRegistry(), toolHeavySession(10), Options{
		ContextWindow: 4000, RecentKeep: 2, ArchiveDir: t.TempDir(),
	}, sink)

	for range 3 {
		if err := a.snipToProjection(context.Background()); err != nil {
			t.Fatalf("snipToProjection: %v", err)
		}
	}
	if len(notices) != 1 {
		t.Errorf("snip announced itself %d times, want 1: %q", len(notices), notices)
	}
}

// preflight's free prune runs the same view and must not undo a summary either.
func TestPreflightPruneKeepsSummaryProjection(t *testing.T) {
	sess := toolHeavySession(14)
	a := New(&fakeProvider{reply: "DIGEST body"}, tool.NewRegistry(), sess, Options{
		ContextWindow: 4000, CompactRatio: 0.5, CompactForceRatio: 0.6, RecentKeep: 2, ArchiveDir: t.TempDir(),
	}, event.Discard)

	if err := a.CompactNow(context.Background(), ""); err != nil {
		t.Fatalf("CompactNow: %v", err)
	}
	if !projectionHasSummary(a) {
		t.Fatal("setup: compaction produced no summary projection")
	}

	if err := a.contextPreflight(context.Background(), CompactionTriggerPressure); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if !projectionHasSummary(a) {
		t.Error("preflight prune discarded the summary projection")
	}
}
