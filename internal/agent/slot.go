package agent

import "context"

// acquireSlot queues this run for a session concurrency/write slot and tells its
// policy the moment one is held. That moment is the only place the wait can be
// read from: everything before it the run spent ready and not running, which no
// participant except the scheduler is in a position to notice.
func (t *TaskTool) acquireSlot(ctx context.Context, req AcquireRequest, sched SchedulerPolicy) (func(), error) {
	if t.scheduler == nil {
		sched.started()
		return func() {}, nil
	}
	release, err := t.scheduler.Acquire(ctx, req)
	if err == nil {
		sched.started()
	}
	return release, err
}

func (p SchedulerPolicy) started() {
	if p.OnStart != nil {
		p.OnStart()
	}
}
