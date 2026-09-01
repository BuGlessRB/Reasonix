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
	lease, err := workspacelease.New(root, config.WorkspaceLeaseDir(), func(w workspacelease.Wait) {
		sink.Emit(workspaceLeaseNotice(w))
	})
	if err != nil {
		return sessionRuntime{}, fmt.Errorf("initialize workspace write lease: %w", err)
	}
	manager := jobs.NewManager(sink, jobOptions...)

	dir := opts.SessionDir
	if dir == "" {
		dir = opts.roots().SessionDir()
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

// workspaceLeaseNotice turns one reported wait into what a frontend can
// resolve. The wait is a warning because the turn is stopped for the length of
// it; its close is not, and carries the measured wait so nothing is left on
// screen still claiming a wait that is over.
func workspaceLeaseNotice(w workspacelease.Wait) event.Event {
	waited := w.Elapsed.Round(100 * time.Millisecond)
	switch w.Outcome {
	case workspacelease.WaitAcquired:
		return event.Event{
			Kind: event.Notice, Level: event.LevelInfo,
			Code:   event.NoticeCodeWorkspaceLeaseResumed,
			Text:   "The workspace is free again; this session has continued.",
			Detail: fmt.Sprintf("waited %s for the workspace write lease", waited),
		}
	case workspacelease.WaitAbandoned:
		return event.Event{
			Kind: event.Notice, Level: event.LevelInfo,
			Code:   event.NoticeCodeWorkspaceLeaseAbandoned,
			Text:   "The wait for the workspace ended before this session's turn to write came.",
			Detail: fmt.Sprintf("waited %s; the turn was cancelled or timed out first", waited),
		}
	default:
		return event.Event{
			Kind: event.Notice, Level: event.LevelWarn,
			Code:   event.NoticeCodeWorkspaceLease,
			Text:   "Another session is writing to this workspace; this session will continue automatically when it is safe.",
			Detail: "workspace write lease is busy; read-only work remains concurrent",
		}
	}
}
