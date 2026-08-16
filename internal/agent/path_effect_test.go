package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
	plan := &toolCallPlan{pathsBefore: snapshotPaths(ledger, dir, []string{scratch})}

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
	decorateObservedPaths(&rec, &toolCallPlan{pathsBefore: snapshotPaths(ledger, dir, nil)})

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

	ledger := evidence.NewLedger()
	cleaned := evidence.Receipt{
		ToolName: "write_file", Success: true, Write: true, Mutation: true,
		MutationEvidence: evidence.MutationProven,
		Paths:            []string{scratch},
		Created:          []string{scratch},
	}
	ledger.Record(cleaned)
	if leftSomethingBehind(ledger, cleaned) {
		t.Fatal("a created file that is now gone left nothing to verify")
	}

	if err := os.WriteFile(kept, []byte("package logstat\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	survivor := cleaned
	survivor.Paths = []string{kept}
	survivor.Created = []string{kept}
	ledger.Record(survivor)
	if !leftSomethingBehind(ledger, survivor) {
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
	if !leftSomethingBehind(evidence.NewLedger(), edit) {
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

// The build-artifact shape: a binary the workspace never named appears, is
// used, and is removed. Watching the top level is what lets the host see both
// halves, so the cleanup does not read as an unverified change.
func TestBuildArtifactCreatedAndRemovedLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "tally-bin")
	ledger := evidence.NewLedger()

	build := &toolCallPlan{pathsBefore: snapshotPaths(ledger, dir, nil)}
	if err := os.WriteFile(artifact, []byte("binary\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	buildRec := evidence.Receipt{ToolName: "bash", Success: true, Mutation: true, MutationEvidence: evidence.MutationUnknown}
	decorateObservedPaths(&buildRec, build)
	ledger.Record(buildRec)
	if !slices.Contains(buildRec.Created, artifact) {
		t.Fatalf("Created = %v, want the artifact the build produced", buildRec.Created)
	}

	cleanup := &toolCallPlan{pathsBefore: snapshotPaths(ledger, dir, nil)}
	if err := os.Remove(artifact); err != nil {
		t.Fatalf("remove: %v", err)
	}
	cleanupRec := evidence.Receipt{ToolName: "bash", Success: true, Mutation: true, MutationEvidence: evidence.MutationUnknown}
	decorateObservedPaths(&cleanupRec, cleanup)
	ledger.Record(cleanupRec)

	if !slices.Contains(cleanupRec.Paths, artifact) {
		t.Fatalf("Paths = %v, want the artifact the cleanup removed", cleanupRec.Paths)
	}
	if leftSomethingBehind(ledger, cleanupRec) {
		t.Fatal("removing what this turn built leaves nothing to verify")
	}
}

// Removing a file the turn did not create is the change, not a cleanup.
func TestRemovingAPreexistingFileStillCounts(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(victim, []byte("k: v\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ledger := evidence.NewLedger()
	plan := &toolCallPlan{pathsBefore: snapshotPaths(ledger, dir, nil)}
	if err := os.Remove(victim); err != nil {
		t.Fatalf("remove: %v", err)
	}
	rec := evidence.Receipt{ToolName: "bash", Success: true, Mutation: true, MutationEvidence: evidence.MutationUnknown}
	decorateObservedPaths(&rec, plan)
	ledger.Record(rec)

	if !leftSomethingBehind(ledger, rec) {
		t.Fatal("deleting a file the turn never created is a change to account for")
	}
}

// A probe written under $TMPDIR is not the work product, so it cannot be what
// the turn owes a verification for. A relative path is the workspace by
// construction — every file tool resolves it there — and must still count.
func TestBaselineIgnoresWritesOutsideTheWorkspace(t *testing.T) {
	root := t.TempDir()
	a := &Agent{}
	a.writeWorkspaceRoot = root

	scratch := evidence.Receipt{
		ToolName: "write_file", Success: true, Write: true, Mutation: true,
		MutationEvidence: evidence.MutationProven,
		Paths:            []string{filepath.Join(t.TempDir(), "probe", "main.go")},
	}
	if a.touchedTheWorkspace(scratch) {
		t.Error("a write outside the workspace was counted as a change to it")
	}

	inside := scratch
	inside.Paths = []string{filepath.Join(root, "internal", "agent", "x.go")}
	if !a.touchedTheWorkspace(inside) {
		t.Error("a write inside the workspace was not counted")
	}

	relative := scratch
	relative.Paths = []string{filepath.Join("internal", "agent", "x.go")}
	if !a.touchedTheWorkspace(relative) {
		t.Error("a relative path resolves against the workspace and must count")
	}

	unnamed := scratch
	unnamed.Paths = nil
	if !a.touchedTheWorkspace(unnamed) {
		t.Error("a change with no named path must be assumed to be the workspace")
	}
}
