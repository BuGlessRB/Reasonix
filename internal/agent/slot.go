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

// acquireRequestFor derives the slot request from the spec, and repairs a
// background writer that arrived without a claim: a caller that assembled the
// spec by hand instead of through buildTaskSpec would otherwise queue against
// nothing and run unserialised against every other writer.
func (t *TaskTool) acquireRequestFor(spec *ProfileExecSpec) (AcquireRequest, error) {
	req := AcquireRequest{
		Writer:     !spec.Grant.ReadOnly,
		WritePaths: spec.Grant.WritePaths,
		Nested:     spec.Sched.Nested,
		Priority:   spec.Sched.Priority,
		Label:      firstNonEmpty(spec.Task.Description, spec.Worker.Name, "task"),
	}
	if !req.Writer || !spec.Grant.WritePaths.Empty() || !spec.Sched.RunInBackground {
		return req, nil
	}
	whole, err := WholeWorkspaceWriteClaim(t.workspaceRoot)
	if err != nil {
		return AcquireRequest{}, err
	}
	req.WritePaths = whole
	spec.Grant.WritePaths = whole
	return req, nil
}

func (p SchedulerPolicy) started() {
	if p.OnStart != nil {
		p.OnStart()
	}
}
