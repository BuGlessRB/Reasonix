package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanMemoryRecallCountsAndPointOfUse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.trajectory.jsonl")
	lines := []string{
		`{"seq":1,"event":{"kind":"tool_result","tool":{"args":"{\"command\":\"make check-fast --tag=MEMKEY-EARLY\"}"}}}`,
		`{"seq":2,"memory_recall":{"hits":[{"id":"a"},{"id":"b"}],"used_chars":420}}`,
		`{"seq":3,"event":{"kind":"tool_result","tool":{"args":"{\"path\":\"answer.txt\",\"content\":\"make check-fast --tag=MEMKEY-USED\"}"}}}`,
		`{"seq":4,"memory_recall":{"suppressed":"generic user turn"}}`,
		`{"seq":5,"event":{"kind":"text","text":"done, MEMKEY-TEXT too"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stats := scanMemoryRecall(path, []string{"MEMKEY-USED", "MEMKEY-TEXT", "MEMKEY-EARLY", "MEMKEY-NEVER"}, false)
	if stats.RecallEvents != 1 || stats.RecallHits != 2 || stats.RecallChars != 420 || stats.Suppressed != 1 {
		t.Fatalf("stats = %+v, want 1 event / 2 hits / 420 chars / 1 suppressed", stats)
	}
	// MEMKEY-EARLY appears only BEFORE the recall: not point-of-use evidence.
	if stats.MarkersUsed != 2 {
		t.Fatalf("markers used = %d, want 2 (post-recall args + answer text only)", stats.MarkersUsed)
	}
}

func TestMemoryUtilitySectionPairsArms(t *testing.T) {
	dir := t.TempDir()
	on := []result{
		{task: task{ID: "helped"}, Passed: true, MemoryRecallEvents: 1, MemoryRecallChars: 300},
		{task: task{ID: "hurt"}, Passed: false, MemoryRecallEvents: 1, MemoryRecallChars: 500},
		{task: task{ID: "same"}, Passed: true, MemoryRecallEvents: 1, MemoryRecallChars: 100},
	}
	off := []result{
		{task: task{ID: "helped"}, Passed: false},
		{task: task{ID: "hurt"}, Passed: true},
		{task: task{ID: "same"}, Passed: true},
	}
	onPath, offPath := filepath.Join(dir, "on.json"), filepath.Join(dir, "off.json")
	for path, rows := range map[string][]result{onPath: on, offPath: off} {
		data, _ := json.Marshal(rows)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	section := memoryUtilitySection(offPath, onPath) // order must not matter
	for _, want := range []string{"Memory utility", "3 paired tasks", "helpful** 1", "harmful** 1", "helped", "hurt"} {
		if !strings.Contains(section, want) {
			t.Fatalf("section missing %q:\n%s", want, section)
		}
	}
}

func TestSeedTaskMemoryBuildsIsolatedStateRoot(t *testing.T) {
	taskDir := t.TempDir()
	work := t.TempDir()
	for _, seed := range []string{"project/fact.md", "global/pref.md"} {
		p := filepath.Join(taskDir, "memory", seed)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("---\nname: x\ndescription: y\n---\n\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stateHome := t.TempDir()
	if err := seedTaskMemory(taskDir, work, stateHome); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateHome, "memory", "global", "pref.md")); err != nil {
		t.Fatalf("global seed missing: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(stateHome, "projects", "*", "memory", "fact.md"))
	if len(matches) != 1 {
		t.Fatalf("project seed not under the work dir's slug: %v", matches)
	}

	if err := seedTaskMemory(t.TempDir(), work, t.TempDir()); err != nil {
		t.Fatalf("task without seeds must be a no-op, got %v", err)
	}
}

// Every task gets a state root of its own, seeded or not. Sharing one leaves
// each finished task's whole event stream on disk under a directory named for
// its work dir, and one observed run found exactly that by grepping the store
// for its own task id while looking for an answer it was not meant to have.
func TestTaskExperimentEnvIsolatesEveryRun(t *testing.T) {
	roots := map[string]bool{}
	for _, id := range []string{"nosol-absent-oracle", "fix-add-bug"} {
		env, drop, note := taskExperimentEnv(suiteConfig{}, task{ID: id, dir: t.TempDir()}, t.TempDir())
		defer drop()
		if note != "" {
			t.Fatalf("%s: %s", id, note)
		}
		root := ""
		for _, e := range env {
			if after, ok := strings.CutPrefix(e, "REASONIX_STATE_HOME="); ok {
				root = after
			}
		}
		if root == "" {
			t.Fatalf("%s ran against the operator's own store root", id)
		}
		if strings.Contains(root, id) {
			t.Errorf("%s: state root %q carries the task id, which is what a run greps for", id, root)
		}
		roots[root] = true
		tmp := ""
		for _, e := range env {
			if after, ok := strings.CutPrefix(e, "TMPDIR="); ok {
				tmp = after
			}
		}
		if tmp == "" {
			t.Errorf("%s inherited the host temp root, where an earlier run's leftovers are still readable", id)
		}
		roots[tmp] = true
	}
	// Two tasks, two roots each, all distinct: a shared one is what lets a
	// no-solution task find the dependency an earlier run compiled for itself.
	if len(roots) != 4 {
		t.Fatalf("two tasks did not get four distinct roots: %v", roots)
	}
}
