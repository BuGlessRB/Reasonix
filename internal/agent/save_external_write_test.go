package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/provider"
	"reasonix/internal/store"
)

// rewriteTranscriptOutsideReasonix replaces what is on disk the way anything
// outside this process would: a sync client resolving its own conflict copy, a
// backup restore, a second install writing through a shared folder. The saved
// session is complete and self-consistent — it simply is not the one the live
// runtime is holding.
func rewriteTranscriptOutsideReasonix(t *testing.T, path string, msgs ...provider.Message) {
	t.Helper()
	// Nothing outside this process goes through Save: it replaces the files,
	// ledger included. Clearing them first is what a restored backup or a
	// resolved sync copy looks like from in here.
	for _, p := range append([]string{path}, store.SessionSidecarFiles(path)...) {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			t.Fatalf("clear %s: %v", filepath.Base(p), err)
		}
	}
	outside := NewSession("sys")
	for _, m := range msgs {
		outside.Add(m)
	}
	if err := outside.Save(path); err != nil {
		t.Fatalf("outside write: %v", err)
	}
}

// A transcript changed underneath a live runtime is the one divergence the
// process cannot prevent: every writer inside it is serialised by snapshotMu
// and guarded across processes by the session lease, so the turns on disk and
// the turns in memory can only stop being prefixes of each other when something
// outside wrote the file.
func TestExternallyRewrittenTranscriptDiverges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	live := NewSession("sys")
	live.Add(provider.Message{Role: provider.RoleUser, Content: "你好"})
	live.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})
	if err := live.Save(path); err != nil {
		t.Fatal(err)
	}

	rewriteTranscriptOutsideReasonix(t, path,
		provider.Message{Role: provider.RoleUser, Content: "你好"},
		provider.Message{Role: provider.RoleAssistant, Content: "别处写进来的回答"})

	live.Add(provider.Message{Role: provider.RoleUser, Content: "本地还没落盘的一轮"})
	err := live.SaveSnapshot(path)
	if !errors.Is(err, ErrSessionSnapshotConflict) {
		t.Fatalf("save = %v, want a conflict after an external rewrite", err)
	}
	if kind, _ := SnapshotConflictKind(err); kind != SessionSnapshotConflictDiverged {
		t.Fatalf("conflict kind = %q, want diverged", kind)
	}
}

// And the shape the sidebar showed: the file keeps being rewritten from
// outside, the runtime keeps holding the same unsaved turn, and every incident
// used to leave its own branch.
func TestRepeatedExternalRewritesShareOneBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	seed := []provider.Message{
		{Role: provider.RoleUser, Content: "你好"},
		{Role: provider.RoleAssistant, Content: "hi"},
	}
	rewriteTranscriptOutsideReasonix(t, path, seed...)

	branches := map[string]bool{}
	for i := range 6 {
		// The runtime is holding the same unsaved turn each time — one
		// conversation, reopened after each incident — while the file keeps
		// being replaced with something new.
		live := NewSession("sys")
		for _, m := range seed {
			live.Add(m)
		}
		live.Add(provider.Message{Role: provider.RoleUser, Content: "没落盘的那一轮"})
		rewriteTranscriptOutsideReasonix(t, path, append(append([]provider.Message(nil), seed...),
			provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("别处写进来的第 %d 版", i)})...)

		if err := live.SaveSnapshot(path); !errors.Is(err, ErrSessionSnapshotConflict) {
			t.Fatalf("incident %d: save = %v, want a conflict", i, err)
		}
		info, err := live.SaveRecoveryBranch(RecoveryBranchOptions{OriginalPath: path})
		if err != nil {
			t.Fatalf("incident %d: %v", i, err)
		}
		branches[info.Path] = true
	}
	if len(branches) != 1 {
		t.Fatalf("six external rewrites over the same unsaved turn produced %d branches, want 1", len(branches))
	}
}
