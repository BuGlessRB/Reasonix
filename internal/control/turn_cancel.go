package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/extension"
	"reasonix/internal/provider"
)

// spawnGuardedTurn launches an admitted turn body plus its autosave companion.
// The caller must already have claimed admission (running=true) under c.mu.
func (c *Controller) spawnGuardedTurn(ctx context.Context, cancel context.CancelFunc, body func(ctx context.Context) error) {
	ctx, completion := withGuardedTurnCompletion(ctx)
	c.autosaveWG.Go(func() {
		c.autosaveWhileRunning(ctx)
	})
	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				c.finishGuardedTurn(fmt.Errorf("internal error: %v", r), completion)
			}
		}()
		err := body(ctx)
		c.finishGuardedTurn(explainError(err), completion)
	}()
}

// finishGuardedTurn keeps admission closed while TurnDone is delivered. The
// sink fan-out may detach per-turn transports; allowing a replacement turn in
// after running=false but before that fan-out completed let the old completion
// clear or inherit the replacement turn's transport.
//
// When the window closes, the oldest parked turn (if any) is started under the
// SAME critical section that clears finishing: opening the gate first and then
// re-admitting would let an unrelated submit slip in ahead and bounce the
// parked turn back to a drop. Remaining parked turns drain one per
// finishGuardedTurn, preserving FIFO order. Rotation cannot interleave here:
// beginRotation refuses while running or finishing, and the drain flips
// finishing directly into running.
func (c *Controller) finishGuardedTurn(err error, completion *guardedTurnCompletion) {
	c.memory.clearAutoRemember()
	c.mu.Lock()
	cancelRequested := c.gate.canceling
	c.gate.running = false
	// A live controller keeps admission closed until TurnDone fan-out finishes.
	// Close has already sealed admission permanently, so a late completion must
	// not resurrect a finishing state after teardown.
	c.gate.finishing = !c.gate.closed
	c.cancel = nil
	c.gate.canceling = false
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.gate.finishing = false
		if c.gate.closed {
			c.mu.Unlock()
			return
		}
		if len(c.parkedTurns) == 0 {
			c.mu.Unlock()
			// No parked compatibility body: admit the next durable inbox item.
			c.maybeDispatchInbox()
			return
		}
		next := c.parkedTurns[0]
		c.parkedTurns = c.parkedTurns[1:]
		ctx, cancel := context.WithCancel(extension.ContextWithRuntimeOwner(context.Background(), c.runtimeOwner))
		c.cancel = cancel
		c.gate.running = true
		c.gate.canceling = false
		c.mu.Unlock()
		c.spawnGuardedTurn(ctx, cancel, next)
	}()
	c.inbox.mu.Lock()
	// Prefer a single representative id for the wire event (first active).
	// Full multi-item ack happens in onInboxTurnDone via activeItemIDs.
	activeInboxID := ""
	for id := range c.inbox.activeItemIDs {
		activeInboxID = id
		break
	}
	c.inbox.mu.Unlock()
	done := event.Event{
		Kind:           event.TurnDone,
		Err:            err,
		Cancelled:      cancelRequested,
		Outcome:        turnOutcome(err),
		CheckpointTurn: c.validatedCheckpointTurn(completion),
		Receipt:        c.executor.CompletionReceipt(),
		ItemID:         activeInboxID,
	}
	var readinessErr *agent.FinalReadinessError
	if errors.As(err, &readinessErr) {
		done.Readiness = &event.FinalReadiness{Attempts: readinessErr.Attempts, Missing: append([]string(nil), readinessErr.Missing...)}
	}
	// Ack active durable items before exposing TurnDone. Frontends commonly
	// refresh the inbox from that event and must not observe already-consumed
	// steers in the completed turn. Dispatch still waits for finishing to clear.
	c.onInboxTurnDone()
	c.sink.Emit(done)
}

// Cancel aborts the in-flight turn. A goroutine blocked awaiting approval
// unblocks via the cancelled context.
func (c *Controller) Cancel() {
	c.mu.Lock()
	cancel := c.cancel
	if cancel != nil {
		c.gate.canceling = true
	}
	c.mu.Unlock()
	if cancel != nil {
		c.approval.clearAll()
		cancel()
		return
	}
	if c.goals.active() {
		c.stopGoal(GoalStatusStopped)
	}
}

// CancelRequested reports whether Cancel has been requested for the active turn.
func (c *Controller) CancelRequested() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gate.canceling
}

// stripInterruptedSyntheticTurnMessagesAfter relocates a synthetic turn after
// an in-turn compaction has rewritten the pre-turn message index, then drops
// that whole controller-created turn.
func (c *Controller) stripInterruptedSyntheticTurnMessagesAfter(idx int) {
	if c.executor == nil {
		return
	}
	msgs := c.executor.Session().Snapshot()
	startedAt := c.inFlightTurnStartedAt()
	if start, ok := resolveInterruptedTurnStart(msgs, idx, false, startedAt, provider.Message{}); ok {
		idx = start
	}
	c.stripTurnMessagesAfter(idx)
}

// stripCancelledVisibleTurnMessagesAfterWithFallback preserves the real user
// prompt and fully paired tool rounds from a cancelled visible turn. Unsafe
// assistant/tool fragments are retained as provider-excluded display history.
// It also covers coordinator
// cancellation before the executor has appended the visible user message. The
// orchestrator owns that input, so it supplies the exact message rather than
// letting cancellation infer the current turn from older transcript history.
func (c *Controller) stripCancelledVisibleTurnMessagesAfterWithFallback(idx int, fallback provider.Message) {
	c.stripCancelledVisibleTurnMessagesAfterWithFallbackAt(idx, fallback, c.inFlightTurnStartedAt())
}

func (c *Controller) stripCancelledVisibleTurnMessagesAfterWithFallbackAt(idx int, fallback provider.Message, startedAt time.Time) {
	if c.executor == nil {
		return
	}
	msgs := c.executor.Session().Snapshot()
	if start, ok := resolveInterruptedTurnStart(msgs, idx, true, startedAt, fallback); ok {
		idx = start
	}
	if idx < 0 {
		idx = 0
	}
	if idx > len(msgs) {
		idx = len(msgs)
	}
	next := append([]provider.Message{}, msgs[:idx]...)
	keptUser := false
	userEnd := idx
	for i, m := range msgs[idx:] {
		if m.Role != provider.RoleUser {
			continue
		}
		if IsSyntheticUserMessage(m.Content) {
			continue
		}
		if _, ok := agent.SteerText(m.Content); ok {
			continue
		}
		m.Content = StripComposePrefixes(m.Content)
		next = append(next, m)
		keptUser = true
		userEnd = idx + i + 1
		break
	}
	if !keptUser && fallback.Role == provider.RoleUser {
		fallback.Content = StripComposePrefixes(fallback.Content)
		if strings.TrimSpace(fallback.Content) != "" {
			fallback.Images = append([]string(nil), fallback.Images...)
			next = append(next, fallback)
			keptUser = true
			userEnd = idx
		}
	}
	if !keptUser && len(msgs) <= idx {
		return
	}
	recovery := &provider.InterruptedTurnRecovery{Pending: true}
	localIndexes := make([]int, 0, 1)
	for i := userEnd; i < len(msgs); {
		m := msgs[i]
		if m.LocalOnly {
			m.Role = provider.RoleTool
			m.ToolCallID = provider.LocalOnlyToolID
			m.Name = provider.LocalOnlyToolName
			m.InterruptedTurn = nil
			m.ToolCalls = displayOnlyToolCalls(m.ToolCalls)
			next = append(next, m)
			localIndexes = append(localIndexes, len(next)-1)
			recovery.DroppedPartialText = recovery.DroppedPartialText || strings.TrimSpace(m.Content) != ""
			recovery.DroppedPartialReasoning = recovery.DroppedPartialReasoning || strings.TrimSpace(m.ReasoningContent) != ""
			for _, call := range m.ToolCalls {
				recovery.InterruptedTools = appendUniqueString(recovery.InterruptedTools, call.Name)
			}
			i++
			continue
		}
		// Auto-compaction can install a digest between the pinned current user
		// message and its recent tool tail. It summarizes pre-turn/current work
		// that is no longer present verbatim, so keep it provider-visible rather
		// than silently dropping context during recovery.
		if agent.IsCompactionSummary(m) {
			next = append(next, m)
			i++
			continue
		}
		if end, ok := completeToolTurnEnd(msgs, i); ok {
			next = append(next, msgs[i:end]...)
			for k, call := range m.ToolCalls {
				if toolResultWasInterrupted(msgs[i+1+k].Content) {
					recovery.InterruptedTools = appendUniqueString(recovery.InterruptedTools, call.Name)
					continue
				}
				recovery.CompletedTools = append(recovery.CompletedTools, interruptedToolSummary(call))
			}
			i = end
			continue
		}
		switch m.Role {
		case provider.RoleAssistant:
			local := m
			local.Role = provider.RoleTool
			local.LocalOnly = true
			local.ToolCallID = provider.LocalOnlyToolID
			local.Name = provider.LocalOnlyToolName
			local.InterruptedTurn = nil
			local.ReasoningSignature = ""
			local.ToolCalls = displayOnlyToolCalls(local.ToolCalls)
			next = append(next, local)
			localIndexes = append(localIndexes, len(next)-1)
			recovery.DroppedPartialText = recovery.DroppedPartialText || strings.TrimSpace(local.Content) != ""
			recovery.DroppedPartialReasoning = recovery.DroppedPartialReasoning || strings.TrimSpace(local.ReasoningContent) != ""
			for _, call := range local.ToolCalls {
				recovery.InterruptedTools = appendUniqueString(recovery.InterruptedTools, call.Name)
			}
		case provider.RoleTool:
			local := m
			local.LocalOnly = true
			local.ToolCalls = []provider.ToolCall{{ID: m.ToolCallID, Name: m.Name}}
			recovery.InterruptedTools = appendUniqueString(recovery.InterruptedTools, m.Name)
			local.ToolCallID = provider.LocalOnlyToolID
			local.Name = provider.LocalOnlyToolName
			next = append(next, local)
			localIndexes = append(localIndexes, len(next)-1)
		}
		i++
	}
	if len(localIndexes) == 0 {
		next = append(next, provider.Message{
			Role: provider.RoleTool, ToolCallID: provider.LocalOnlyToolID,
			Name: provider.LocalOnlyToolName, LocalOnly: true,
		})
		localIndexes = append(localIndexes, len(next)-1)
	}
	next[localIndexes[len(localIndexes)-1]].InterruptedTurn = recovery
	c.replaceSessionAfterCancel(next)
}

func (c *Controller) hasInterruptedDisplayAfter(idx int, fallback provider.Message) bool {
	if c.executor == nil {
		return false
	}
	msgs := c.executor.Session().Snapshot()
	if start, ok := resolveInterruptedTurnStart(msgs, idx, true, c.inFlightTurnStartedAt(), fallback); ok {
		idx = start
	}
	idx = max(0, min(idx, len(msgs)))
	for _, m := range msgs[idx:] {
		if m.LocalOnly && m.InterruptedTurn != nil {
			return true
		}
	}
	return false
}

func (c *Controller) replaceSessionAfterCancel(msgs []provider.Message) {
	// The whole cleanup is a save/recovery handoff like snapshot's: hold
	// snapshotMu from the in-memory truncation onward. Truncating outside the
	// lock would let an in-flight save capture the shortened transcript, read
	// the longer partial autosave on disk as a stale-prefix conflict, and
	// adopt it back into the executor — silently undoing the cancel cleanup
	// before the flush below could persist it.
	c.snapshotMu.Lock()
	defer c.snapshotMu.Unlock()
	c.executor.Session().Replace(append([]provider.Message(nil), msgs...))
	// Rebuild canonical todo state from the truncated transcript so
	// Controller.Todos(), goal readiness, and the task panel no longer see
	// the in_progress items written by the cancelled turn.
	c.executor.RebuildTodoState()
	// The mid-turn autosave may have already written a partial transcript to
	// disk. snapshotActivityIfChanged skips the write when messageCount()
	// returns to startMessages, so flush the cleaned transcript here. SaveRewrite
	// still checks that this controller owns the current on-disk baseline before
	// overwriting it, and also covers the edge case where the strip leaves only a
	// system message (HasContent() == false). The path is read under the lock so
	// an in-flight recovery retarget cannot leave it stale.
	c.mu.Lock()
	path := c.sessionPath
	c.mu.Unlock()
	if path != "" {
		if err := c.executor.Session().SaveRewrite(path); err != nil {
			if errors.Is(err, agent.ErrSessionSnapshotConflict) {
				if _, outcome, recoverErr := c.recoverSnapshotConflict(path, err, true); recoverErr != nil {
					slog.Warn("controller: post-cancel transcript recovery", "err", recoverErr)
				} else if outcome == conflictDropped {
					slog.Warn("controller: post-cancel transcript dropped after conflict", "path", path)
				}
			} else {
				slog.Warn("controller: post-cancel transcript flush", "err", err)
			}
		}
	}
}
