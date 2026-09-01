package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/store"
)

func TestPinnedContextSidecarRoundTripAndCopy(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jsonl")
	target := filepath.Join(dir, "target.jsonl")
	files := []string{"docs/b.md", "docs/a.md", "docs/b.md"}
	if err := savePinnedContextState(source, files); err != nil {
		t.Fatalf("save source: %v", err)
	}
	state, err := loadPinnedContextState(source)
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	want := []string{"docs/a.md", "docs/b.md"}
	if !reflect.DeepEqual(state.Files, want) {
		t.Fatalf("source files = %v, want stable sorted order %v", state.Files, want)
	}
	if state.SchemaVersion != pinnedContextSchemaVersion || state.SessionID != agent.BranchID(source) {
		t.Fatalf("source state = %+v", state)
	}
	if err := copyPinnedContextState(source, target); err != nil {
		t.Fatalf("copy sidecar: %v", err)
	}
	copied, err := loadPinnedContextState(target)
	if err != nil {
		t.Fatalf("load copied sidecar: %v", err)
	}
	if copied.SessionID != agent.BranchID(target) || !reflect.DeepEqual(copied.Files, want) {
		t.Fatalf("copied state = %+v", copied)
	}
}

func TestCopyPinnedContextStateCopiesEmptySourceSemantics(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jsonl")
	target := filepath.Join(dir, "target.jsonl")
	if err := savePinnedContextState(target, []string{"stale.md"}); err != nil {
		t.Fatal(err)
	}
	if err := copyPinnedContextState(source, target); err != nil {
		t.Fatal(err)
	}
	state, err := loadPinnedContextState(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 0 {
		t.Fatalf("empty source left stale target pins: %v", state.Files)
	}
}

func TestPinnedContextSidecarRejectsCorruptionAndUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	sidecar := store.SessionPinnedContext(path)
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "corrupt", raw: "{"},
		{name: "unknown schema", raw: `{"schemaVersion":2,"sessionId":"session","files":[]}`},
		{name: "wrong session", raw: `{"schemaVersion":1,"sessionId":"other","files":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(sidecar, []byte(test.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadPinnedContextState(path); err == nil {
				t.Fatal("invalid sidecar was accepted")
			}
		})
	}
}

func TestLegacyPinnedFilesMigrateOnlyWhenSidecarAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := savePinnedContextState(path, []string{}); err != nil {
		t.Fatal(err)
	}
	state, err := loadOrMigratePinnedContextState(path, []string{"legacy.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 0 {
		t.Fatalf("legacy pins overwrote existing sidecar: %v", state.Files)
	}
	raw, err := os.ReadFile(store.SessionPinnedContext(path))
	if err != nil {
		t.Fatal(err)
	}
	var persisted pinnedContextState
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Files) != 0 {
		t.Fatalf("persisted files = %v, want empty", persisted.Files)
	}
}
