package main

import (
	"fmt"
	"os/exec"
	"slices"
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

// The invariant the whole instantiation layer exists for: no literal a scorer
// needs may be readable in this repository before a run starts.
func TestNoAnswerLiteralExistsInTheRepository(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skip(err)
	}
	for _, task := range allTasks() {
		inst, err := instantiateTask(task, seededRand(task.ID, "leak-check"))
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range append(inst.AnswerMarkers, inst.CueMarker) {
			if len(marker) < 6 {
				continue // a bare number is not a literal anyone can grep for
			}
			if found, where := grepRepo(t, root, marker); found {
				t.Errorf("%s: %q is readable at %s before the run", task.ID, marker, where)
			}
		}
	}
}

// Two runs of the same task must not share an answer, or one leaked run leaks
// every future one.
func TestInstancesDifferBetweenRuns(t *testing.T) {
	for _, task := range allTasks() {
		a, err := instantiateTask(task, liveRand())
		if err != nil {
			t.Fatal(err)
		}
		b, err := instantiateTask(task, liveRand())
		if err != nil {
			t.Fatal(err)
		}
		if slices.Equal(a.AnswerMarkers, b.AnswerMarkers) {
			t.Errorf("%s produced the same answers twice: %v", task.ID, a.AnswerMarkers)
		}
	}
}

// A preflight that could not reproduce would calibrate against one run's
// nonces and report about another's.
func TestSeededInstancesReproduce(t *testing.T) {
	for _, task := range allTasks() {
		a, _ := instantiateTask(task, seededRand(task.ID, "x"))
		b, _ := instantiateTask(task, seededRand(task.ID, "x"))
		if !slices.Equal(a.AnswerMarkers, b.AnswerMarkers) || a.body != b.body {
			t.Errorf("%s did not reproduce under the same seed", task.ID)
		}
	}
}

// An unresolved placeholder would put "{{marker}}" in the transcript and score
// every run wrong; it fails the fixture instead.
func TestUnresolvedPlaceholderIsRefused(t *testing.T) {
	if _, err := instantiate("value is {{missing}}", fixtureVars{"other": "x"}); err == nil {
		t.Fatal("an unresolved placeholder was accepted")
	}
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git checkout: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// grepRepo searches tracked files only: build artifacts and temp fixtures are
// not what a fresh checkout hands anyone.
func grepRepo(t *testing.T, root, needle string) (bool, string) {
	t.Helper()
	cmd := exec.Command("git", "grep", "-l", "-F", needle)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false, "" // exit 1 means no match
	}
	return true, strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
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
