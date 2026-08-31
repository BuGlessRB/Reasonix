package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/repair"
)

func withProbation(t *testing.T, d time.Duration) {
	t.Helper()
	original := updateProbation
	updateProbation = d
	t.Cleanup(func() { updateProbation = original })
}

func withCommitSeam(t *testing.T) chan string {
	t.Helper()
	original := commitUpdateHealth
	committed := make(chan string, 1)
	commitUpdateHealth = func(_ *repair.UpdateHealthWitness, running string) error {
		committed <- running
		return nil
	}
	t.Cleanup(func() { commitUpdateHealth = original })
	return committed
}

func TestReadyWindowCommitsAfterProbation(t *testing.T) {
	withProbation(t, time.Millisecond)
	committed := withCommitSeam(t)
	app := &App{updateHealth: &repair.UpdateHealthWitness{}}

	app.acknowledgeUpdateHealth(context.Background())

	select {
	case running := <-committed:
		if running != version {
			t.Fatalf("committed for %q, want the running build %q", running, version)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a window that survived probation never committed the update it booted from")
	}
}

// A build that dies inside probation has shown nothing. Committing there would
// retire the rollback material of the update that killed it.
func TestWindowLostInsideProbationCommitsNothing(t *testing.T) {
	withProbation(t, time.Hour)
	committed := withCommitSeam(t)
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{updateHealth: &repair.UpdateHealthWitness{}}

	app.acknowledgeUpdateHealth(ctx)
	cancel()

	select {
	case running := <-committed:
		t.Fatalf("a window lost inside probation committed for %q", running)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestLaunchWithoutAnUpdateCommitsNothing(t *testing.T) {
	withProbation(t, time.Millisecond)
	committed := withCommitSeam(t)

	(&App{}).acknowledgeUpdateHealth(context.Background())

	select {
	case running := <-committed:
		t.Fatalf("a launch that booted from no update committed for %q", running)
	case <-time.After(100 * time.Millisecond):
	}
}
