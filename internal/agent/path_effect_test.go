package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/evidence"
)

func TestObservationNamesWhatAnOpaqueCommandTouched(t *testing.T) {
	dir := t.TempDir()
	edited := filepath.Join(dir, "parse.go")
	scratch := filepath.Join(dir, "probe.go")
	if err := os.WriteFile(edited, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{edited}})
	plan := &toolCallPlan{pathsBefore: snapshotPaths(ledger, []string{scratch})}

	// What `sed -i parse.go` and a scratch file would do, without the host
	// knowing either command.
	if err := os.WriteFile(edited, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(scratch, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := evidence.Receipt{ToolName: "bash", Success: true, Mutation: true, MutationEvidence: evidence.MutationUnknown}
	decorateObservedPaths(&rec, plan)

	if rec.MutationEvidence != evidence.MutationProven {
		t.Fatalf("MutationEvidence = %q, want proven", rec.MutationEvidence)
	}
	if len(rec.Paths) != 2 {
		t.Fatalf("Paths = %v, want both touched files", rec.Paths)
	}
	if len(rec.Created) != 1 || rec.Created[0] != scratch {
		t.Fatalf("Created = %v, want only the new file", rec.Created)
	}
}

// A call that changed nothing among the watched paths may still have changed
// something outside them, so it must not be downgraded to "changed nothing".
func TestObservationNeverDowngradesAnUnknownCall(t *testing.T) {
	dir := t.TempDir()
	quiet := filepath.Join(dir, "untouched.go")
	if err := os.WriteFile(quiet, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{quiet}})

	rec := evidence.Receipt{ToolName: "bash", Success: true, Mutation: true, MutationEvidence: evidence.MutationUnknown}
	decorateObservedPaths(&rec, &toolCallPlan{pathsBefore: snapshotPaths(ledger, nil)})

	if rec.MutationEvidence != evidence.MutationUnknown {
		t.Fatalf("MutationEvidence = %q, want it left unknown", rec.MutationEvidence)
	}
}

// The investigation shape: a scratch file written, used, and removed leaves
// nothing to verify, while an edit that is still on disk does.
func TestBaselineIgnoresScratchFilesTheTurnCleanedUp(t *testing.T) {
	dir := t.TempDir()
	scratch := filepath.Join(dir, "zz_probe.go")
	kept := filepath.Join(dir, "stats.go")

	cleaned := evidence.Receipt{
		ToolName: "write_file", Success: true, Write: true, Mutation: true,
		MutationEvidence: evidence.MutationProven,
		Paths:            []string{scratch},
		Created:          []string{scratch},
	}
	if leftSomethingBehind(cleaned) {
		t.Fatal("a created file that is now gone left nothing to verify")
	}

	if err := os.WriteFile(kept, []byte("package logstat\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	survivor := cleaned
	survivor.Paths = []string{kept}
	survivor.Created = []string{kept}
	if !leftSomethingBehind(survivor) {
		t.Fatal("a created file still on disk must be verified")
	}
}

// An edit to a file that already existed is never "cleaned up", even if the
// path later becomes unreadable: the host only vouches for what it watched
// appear.
func TestBaselineKeepsWritesItDidNotWatchCreate(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "never-existed.go")
	edit := evidence.Receipt{
		ToolName: "edit_file", Success: true, Write: true, Mutation: true,
		MutationEvidence: evidence.MutationProven,
		Paths:            []string{gone},
	}
	if !leftSomethingBehind(edit) {
		t.Fatal("a write with no observed creation must count as surviving")
	}
}

func TestObservedPathsSurviveReceiptRoundTrip(t *testing.T) {
	rec := evidence.Receipt{ToolName: "bash", Created: []string{"a.go"}}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back evidence.Receipt
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Created) != 1 || back.Created[0] != "a.go" {
		t.Fatalf("Created = %v after round trip", back.Created)
	}
}
