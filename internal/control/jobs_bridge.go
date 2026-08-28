// jobs_bridge.go — a terminal background job becomes a turn the session runs.
package control

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"reasonix/internal/jobs"
	"reasonix/internal/sessioninbox"
	"reasonix/internal/taskmonitor"
)

// hostContinuationSource labels inbox items the runtime authored for itself.
const hostContinuationSource = "host:background-job"

// observeJobs attaches what watches the background-job table: the task-store
// recorder, and this controller as the completion observer. The recorder
// swallows its own failures, and resolves the session id lazily because the
// session path is only fixed once the first turn begins.
func (c *Controller) observeJobs(taskStore taskmonitor.WriteStore) {
	if c.jobs == nil {
		return
	}
	if c.workspaceRoot != "" {
		if taskStore == nil {
			taskStore = taskmonitor.NewFileStore(filepath.Join(".reasonix", "tasks"))
		}
		c.jobs.SetTaskRecorder(taskmonitor.NewTaskRecorder(taskStore, c.workspaceRoot,
			func() string { return c.parentSessionID() }))
	}
	// The manager keeps its own note either way, so a controller that cannot
	// take an event costs nothing.
	c.jobs.SetCompletionObserver(c)
}

// OnJobCompletion implements jobs.CompletionObserver: a terminal background job
// gives its session a turn instead of waiting for the user's next message. An
// error means the event never reached the durable queue, and the manager's own
// note carries it into the next real turn instead.
func (c *Controller) OnJobCompletion(_ context.Context, ev jobs.CompletionEvent) error {
	if c == nil {
		return fmt.Errorf("control: no controller for job %s", ev.JobID)
	}
	// Jobs owned by a session that is not the active one keep their queued note
	// until that session is active again — the same rule the drain path applies.
	if session := strings.TrimSpace(ev.SessionID); session != "" && session != c.parentSessionID() {
		return fmt.Errorf("control: job %s belongs to another session", ev.JobID)
	}
	// A run with no persisted session (headless `run`, e2ebench) has no durable
	// inbox to own the continuation, and must degrade rather than fail.
	if c.SessionPath() == "" {
		return fmt.Errorf("control: session has no durable inbox")
	}
	text := backgroundCompletionPrompt(ev)
	if text == "" {
		return fmt.Errorf("control: job %s produced no continuation", ev.JobID)
	}
	_, err := c.EnqueueHostContinuation(InboxRequest{
		Display:     text,
		Submit:      text,
		Source:      hostContinuationSource,
		Idempotency: ev.ID,
	})
	return err
}

// EnqueueHostContinuation durably queues runtime-authored work and delivers it
// into the live turn, or as the next one. Which of the two is never decided by
// reading whether a turn is running — the turn can end in that gap — but by the
// steer attempt itself, whose refusal converts the item and kicks the
// dispatcher. A nil error means durable, not read.
func (c *Controller) EnqueueHostContinuation(req InboxRequest) (sessioninbox.InboxReceipt, error) {
	req.Intent = sessioninbox.IntentSteer
	req.Origin = sessioninbox.OriginHost
	rec, err := c.EnqueueInbox(req)
	if err != nil {
		return rec, err
	}
	if rec.Idempotent {
		return rec, nil
	}
	steered, steerErr := c.TrySteerInboxItem(rec.ItemID)
	if steerErr != nil {
		// Durable and still queued: delivery is the dispatcher's now.
		c.maybeDispatchInbox()
		return rec, nil
	}
	return steered, nil
}

// backgroundCompletionPrompt is the host's own account of what finished. It
// states the outcome and leaves to the model whether anything is unblocked.
func backgroundCompletionPrompt(ev jobs.CompletionEvent) string {
	tag := ev.JobID
	if ev.Label != "" {
		tag = fmt.Sprintf("%s (%s)", ev.JobID, ev.Label)
	}
	if tag == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("<background-jobs>\n")
	fmt.Fprintf(&b, "%s — %s\n", tag, ev.Status)
	b.WriteString("</background-jobs>\n\n")
	b.WriteString("A background job you started has finished. Read its result with wait or bash_output, then continue the work it was part of. Do not redo what it already did, and end the turn if nothing is left to do.")
	return b.String()
}
