package agent

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerTotalConcurrencyQueues(t *testing.T) {
	s := NewSubagentScheduler(2, 2)
	root := t.TempDir()
	var started atomic.Int32
	var max atomic.Int32
	var wg sync.WaitGroup
	barrier := make(chan struct{})

	for range 4 {
		wg.Go(func() {
			release, err := s.Acquire(context.Background(), AcquireRequest{Writer: false})
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			cur := started.Add(1)
			for {
				old := max.Load()
				if cur <= old || max.CompareAndSwap(old, cur) {
					break
				}
			}
			<-barrier
			started.Add(-1)
			release()
		})
	}

	// Wait until at least 2 are running, then release them.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if max.Load() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := max.Load(); got > 2 {
		t.Fatalf("max concurrent = %d, want <= 2", got)
	}
	close(barrier)
	wg.Wait()
	_ = root
}

func TestSchedulerNestedFailsFast(t *testing.T) {
	s := NewSubagentScheduler(1, 1)
	release, err := s.Acquire(context.Background(), AcquireRequest{Writer: false})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = s.Acquire(context.Background(), AcquireRequest{Writer: false, Nested: true})
	if err == nil {
		t.Fatal("nested acquire should fail fast at limit")
	}
}

func TestSchedulerWriterPathConflictQueues(t *testing.T) {
	s := NewSubagentScheduler(4, 2)
	root := t.TempDir()
	claim, err := NormalizeWritePaths(root, []string{"a.md"})
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.Acquire(context.Background(), AcquireRequest{Writer: true, WritePaths: claim})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// Same path cannot start while the first claim is held — with Nested it fails.
	_, err = s.Acquire(ctx, AcquireRequest{Writer: true, WritePaths: claim, Nested: true})
	if err == nil {
		t.Fatal("expected path conflict for nested acquire")
	}
	release()

	// After release, same path is free.
	release2, err := s.Acquire(context.Background(), AcquireRequest{Writer: true, WritePaths: claim})
	if err != nil {
		t.Fatal(err)
	}
	release2()
}

func TestSchedulerTryClaimWritePaths(t *testing.T) {
	s := NewSubagentScheduler(4, 2)
	root := t.TempDir()
	claim, _ := NormalizeWritePaths(root, []string{"a.md"})
	release, err := s.Acquire(context.Background(), AcquireRequest{Writer: true, WritePaths: claim})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := s.TryClaimWritePaths(claim); err == nil {
		t.Fatal("parent should see active claim")
	}
	other, _ := NormalizeWritePaths(root, []string{"b.md"})
	if err := s.TryClaimWritePaths(other); err != nil {
		t.Fatalf("disjoint claim should be free: %v", err)
	}
}

// waitForQueuedAcquires blocks until exactly want acquires are parked in the
// queue, so a test can prove which one a freed slot goes to instead of racing
// the goroutines that ask for it.
func waitForQueuedAcquires(t *testing.T, s *SubagentScheduler, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := len(s.waiters)
		s.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waited for %d queued acquires and never saw them", want)
}

// A slot used to go to whoever asked first, which on a dependency graph is how
// the head of the longest chain ends up behind leaves that unblock nothing. The
// wait lands on the run's makespan one for one, and no edge on the graph can
// draw it.
func TestSchedulerGivesAFreedSlotToTheHeaviestWaiter(t *testing.T) {
	s := NewSubagentScheduler(1, 1)
	held, err := s.Acquire(context.Background(), AcquireRequest{})
	if err != nil {
		t.Fatal(err)
	}

	granted := make(chan int, 2)
	var wg sync.WaitGroup
	// Queue the leaf first so arrival order and weight disagree.
	for i, priority := range []int{0, 4} {
		wg.Go(func() {
			release, err := s.Acquire(context.Background(), AcquireRequest{Priority: priority})
			if err != nil {
				t.Errorf("acquire priority %d: %v", priority, err)
				return
			}
			granted <- priority
			release()
		})
		waitForQueuedAcquires(t, s, i+1)
	}

	held()
	wg.Wait()
	close(granted)
	var order []int
	for p := range granted {
		order = append(order, p)
	}
	if want := []int{4, 0}; !slices.Equal(order, want) {
		t.Fatalf("slots went out in priority order %v, want %v", order, want)
	}
}
