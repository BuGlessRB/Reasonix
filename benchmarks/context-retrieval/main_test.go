package main

import (
	"strings"
	"testing"
)

// The same checks the preflight mode runs, under go test: a compaction change
// that invalidates the fixtures fails here rather than three hundred API calls
// later.

func TestCorpusIsWellFormed(t *testing.T) {
	if err := validateCorpus(); err != nil {
		t.Fatal(err)
	}
	if got := len(searchTasks()); got != 6 {
		t.Errorf("search experiment has %d tasks, want 6", got)
	}
	if got := len(indexTasks()); got != 6 {
		t.Errorf("index experiment has %d tasks, want 6", got)
	}
	tiers := map[string]int{}
	for _, task := range indexTasks() {
		tiers[task.CueTier]++
	}
	// Two per tier is what makes the staircase readable: the same task can be
	// compared across the boundary its cue sits on, instead of two different
	// questions being compared across two arms.
	for _, tier := range []string{tierQuarter, tierHalf, tierDefault} {
		if tiers[tier] != 2 {
			t.Errorf("tier %s has %d tasks, want 2", tier, tiers[tier])
		}
	}
}

// Nonces, not vocabulary: a marker a model could produce from general knowledge
// scores a recall it never performed.
func TestAnswerMarkersAreDistinctive(t *testing.T) {
	seen := map[string]string{}
	for _, task := range allTasks() {
		distinctive := 0
		for _, m := range task.AnswerMarkers {
			if len(m) >= 6 && strings.ContainsAny(m, "-0123456789") {
				distinctive++
			}
			if other, ok := seen[m]; ok && other != task.ID && len(m) >= 6 {
				t.Errorf("%s and %s share marker %q", other, task.ID, m)
			}
			seen[m] = task.ID
		}
		if distinctive == 0 {
			t.Errorf("%s has no distinctive marker: %v", task.ID, task.AnswerMarkers)
		}
	}
}

func TestEveryFixturePassesPreflight(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and folds 36 sessions")
	}
	root := t.TempDir()
	for _, task := range allTasks() {
		t.Run(task.ID, func(t *testing.T) {
			found, err := checkTask(task, root)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range found {
				t.Error(f)
			}
		})
	}
}
