package agent

import (
	"fmt"
	"path/filepath"
	"testing"

	"reasonix/internal/provider"
)

// staleSessionOver returns a session holding the parent transcript plus one
// unsaved message, which is what a snapshot conflict has in memory.
func staleSessionOver(parent []provider.Message, extra string) *Session {
	s := NewSession("sys")
	for _, m := range parent {
		if m.Role == provider.RoleSystem {
			continue
		}
		s.Add(m)
	}
	s.Add(provider.Message{Role: provider.RoleUser, Content: extra})
	return s
}

// Recovery lanes are per live Session, so a conflict from a fresh Session is a
// fresh file. The depth cap bounds the chain (parent → branch → branch), never
// how many siblings one parent may accumulate — and every resume, rebuild or
// reopened pane brings a new Session. The report that prompted this test was a
// sidebar holding dozens of rows with one title and one turn count.
func TestRepeatedConflictsFromFreshSessionsDoNotFanOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	parent := NewSession("sys")
	parent.Add(provider.Message{Role: provider.RoleUser, Content: "你好"})
	parent.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})
	if err := parent.Save(path); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	parentMsgs := parent.Snapshot()

	paths := map[string]bool{}
	for i := range 12 {
		// The same unsaved turns every time: one conversation reopened again
		// and again, which is what puts a dozen identical rows in the sidebar.
		stale := staleSessionOver(parentMsgs, "unsaved")
		info, err := stale.SaveRecoveryBranch(RecoveryBranchOptions{OriginalPath: path})
		if err != nil {
			t.Fatalf("conflict %d: %v", i, err)
		}
		paths[info.Path] = true
	}
	if len(paths) > 1 {
		t.Errorf("12 conflicts over the same content produced %d recovery branches; one parent should not accumulate identical siblings", len(paths))
	}
}

// Collapsing identical content must not collapse different content: a branch
// exists to keep turns that are nowhere else, and two conflicts carrying
// different unsaved work are two things to keep.
func TestDistinctConflictContentStillGetsItsOwnBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	parent := NewSession("sys")
	parent.Add(provider.Message{Role: provider.RoleUser, Content: "你好"})
	parent.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})
	if err := parent.Save(path); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	parentMsgs := parent.Snapshot()

	paths := map[string]bool{}
	for i := range 3 {
		stale := staleSessionOver(parentMsgs, fmt.Sprintf("unsaved %d", i))
		info, err := stale.SaveRecoveryBranch(RecoveryBranchOptions{OriginalPath: path})
		if err != nil {
			t.Fatalf("conflict %d: %v", i, err)
		}
		paths[info.Path] = true
	}
	if len(paths) != 3 {
		t.Errorf("three different unsaved transcripts landed in %d branches; each carries turns the others do not", len(paths))
	}
}

// The same content arriving twice must not become two files: a Session that
// reloads and conflicts again on an unchanged transcript is the common case,
// and it is the one that fills a sidebar fastest.
func TestIdenticalConflictContentReusesOneBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	parent := NewSession("sys")
	parent.Add(provider.Message{Role: provider.RoleUser, Content: "你好"})
	parent.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})
	if err := parent.Save(path); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	parentMsgs := parent.Snapshot()

	first, err := staleSessionOver(parentMsgs, "same").SaveRecoveryBranch(RecoveryBranchOptions{OriginalPath: path})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := staleSessionOver(parentMsgs, "same").SaveRecoveryBranch(RecoveryBranchOptions{OriginalPath: path})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.Path != second.Path {
		t.Errorf("identical content wrote two branches:\n  %s\n  %s", first.Path, second.Path)
	}
}
