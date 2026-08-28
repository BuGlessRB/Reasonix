package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/testenv"
	"reasonix/internal/tool"
)

// A hand-typed /compact on a transcript with nothing to fold used to answer with
// ErrCompactionRequired — "context exceeds provider limit and compaction failed"
// — on a session nowhere near the limit. Manual carries Force, and the no-fold
// branch read Force as overflow.
func TestManualCompactWithNothingToFoldIsAVerdictNotAnOverflow(t *testing.T) {
	shapes := map[string][]provider.Message{
		"one exchange": {
			{Role: provider.RoleSystem, Content: "sys"},
			{Role: provider.RoleUser, Content: "hello"},
			{Role: provider.RoleAssistant, Content: "hi"},
		},
		"nothing answered yet": {
			{Role: provider.RoleSystem, Content: "sys"},
			{Role: provider.RoleUser, Content: "hello"},
		},
		"system only": {
			{Role: provider.RoleSystem, Content: "sys"},
		},
	}
	for name, msgs := range shapes {
		t.Run(name, func(t *testing.T) {
			a := New(&fakeProvider{reply: "unused"}, tool.NewRegistry(), &Session{Messages: msgs}, Options{
				ContextWindow: 200_000, CompactRatio: 0.8, RecentKeep: 2,
				SessionPath: filepath.Join(testenv.TempDir(t), "session.jsonl"),
				WorkspaceID: "ws", ModelRef: "p/m",
			}, event.Discard)

			err := a.CompactNow(context.Background(), "")
			if err == nil {
				return // folding a short transcript is allowed to succeed
			}
			if !IsCompactionDeclined(err) {
				t.Fatalf("CompactNow = %v, want a declined verdict", err)
			}
			// The window is 200k and the transcript is three lines: nothing here
			// may claim the provider limit was reached.
			if strings.Contains(err.Error(), ErrCompactionRequired.Error()) {
				t.Fatalf("CompactNow = %v, want no claim about the provider limit", err)
			}
			if CompactionDeclineReason(err) == "" {
				t.Fatalf("CompactNow = %v, want a reason a frontend can show", err)
			}
		})
	}
}
