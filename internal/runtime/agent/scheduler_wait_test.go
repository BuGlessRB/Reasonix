package agent

import (
	"testing"
	"time"

	"reasonix/internal/runtime/writeclaim"
)

// waitForQueuedAcquires blocks until exactly want acquires are parked in the
// queue, so a test can prove which one a freed slot goes to instead of racing
// the goroutines that ask for it.
func waitForQueuedAcquires(t *testing.T, s *writeclaim.SubagentScheduler, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.Queued() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waited for %d queued acquires and never saw them", want)
}
