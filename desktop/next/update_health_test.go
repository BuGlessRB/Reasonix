package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/serve"
)

type stubUpdateHost struct{}

func (stubUpdateHost) AcknowledgeLaunchHealth() error { return nil }

func withProbation(t *testing.T, d time.Duration) {
	t.Helper()
	original := updateProbation
	updateProbation = d
	t.Cleanup(func() { updateProbation = original })
}

func withCommitSeam(t *testing.T) chan struct{} {
	t.Helper()
	original := commitUpdateHealth
	committed := make(chan struct{}, 1)
	commitUpdateHealth = func(serve.UpdateHost) error {
		committed <- struct{}{}
		return nil
	}
	t.Cleanup(func() { commitUpdateHealth = original })
	return committed
}

// The window supplies the timing and nothing else: which transaction this
// retires was read by the host at startup, so there is no version here to
// assert on any more, and none for a shell to get wrong.
func TestReadyWindowCommitsAfterProbation(t *testing.T) {
	withProbation(t, time.Millisecond)
	committed := withCommitSeam(t)
	app := &App{updateHost: stubUpdateHost{}}

	app.acknowledgeUpdateHealth(context.Background())

	select {
	case <-committed:
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
	app := &App{updateHost: stubUpdateHost{}}

	app.acknowledgeUpdateHealth(ctx)
	cancel()

	select {
	case <-committed:
		t.Fatal("a window lost inside probation committed")
	case <-time.After(100 * time.Millisecond):
	}
}

// A shell with no capability behind it has nothing to acknowledge through, and
// must not invent one. What a launch that booted from no update retires is the
// host's question, and it answers it by retiring nothing.
func TestAShellWithNoUpdateCapabilityCommitsNothing(t *testing.T) {
	withProbation(t, time.Millisecond)
	committed := withCommitSeam(t)

	(&App{}).acknowledgeUpdateHealth(context.Background())

	select {
	case <-committed:
		t.Fatal("a shell owning no update capability committed")
	case <-time.After(100 * time.Millisecond):
	}
}
