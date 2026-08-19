package boot

import (
	"fmt"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/jobs"
	"reasonix/internal/workspacelease"
)

// sessionRuntime is the session-scoped machinery a build hands to the
// controller: the workspace write lease, the background job manager that
// retains it while a job runs, and the directory sessions are read from.
type sessionRuntime struct {
	lease *workspacelease.Owner
	jobs  *jobs.Manager
	dir   string
}

// startSessionRuntime acquires the workspace lease and opens the job manager.
// Every role setting lazily acquires the lease on the first real writer, so
// read-only turns never take it.
func startSessionRuntime(opts Options, cfg *config.Config, root string, sink event.Sink) (sessionRuntime, error) {
	jobOptions := []jobs.Option{
		jobs.WithStalledWarningAfter(time.Duration(cfg.BackgroundJobStalledWarningSeconds()) * time.Second),
		jobs.WithSessionOwnershipProbe(agent.SessionLeaseHeldByCurrentRuntime),
	}
	lease, err := workspacelease.New(root, config.WorkspaceLeaseDir(), func() {
		sink.Emit(event.Event{
			Kind:   event.Notice,
			Level:  event.LevelInfo,
			Code:   event.NoticeCodeWorkspaceLease,
			Text:   "Another session is writing to this workspace; this session will continue automatically when it is safe.",
			Detail: "workspace write lease is busy; read-only work remains concurrent",
		})
	})
	if err != nil {
		return sessionRuntime{}, fmt.Errorf("initialize workspace write lease: %w", err)
	}
	jobOptions = append(jobOptions, jobs.WithJobStartObserver(lease.RetainUntil))
	manager := jobs.NewManager(sink, jobOptions...)

	dir := opts.SessionDir
	if dir == "" {
		dir = config.SessionDir()
	}
	reconcileCleanupPending := opts.CleanupPendingReconciler
	if reconcileCleanupPending == nil {
		reconcileCleanupPending = control.ReconcileCleanupPending
	}
	if err := reconcileCleanupPending(dir); err != nil {
		report(sink, event.Event{Level: event.LevelWarn, Text: "cleanup-pending reconciliation failed: " + err.Error()})
	}
	return sessionRuntime{lease: lease, jobs: manager, dir: dir}, nil
}
