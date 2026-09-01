package sessioncatalog

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"strings"
	"time"

	"reasonix/internal/agent"
)

const repairWakeKey = "session-catalog-repair-wake"

type repairItem struct {
	path             string
	pathKey          string
	target           DirectoryTarget
	topicID          string
	workspaceRootKey string
	attempts         int
	state            string
}

type repairOutcome struct {
	item   repairItem
	result agent.SessionListingRepairResult
	err    error
}

func (c *Catalog) enqueueRepair(path string) {
	if c == nil || c.opts.DisableRepair || strings.TrimSpace(path) == "" {
		return
	}
	if _, loaded := c.repairQueued.LoadOrStore(repairWakeKey, struct{}{}); loaded {
		return
	}
	select {
	case c.repairCh <- path:
	case <-c.stop:
		c.repairQueued.Delete(repairWakeKey)
	default:
		c.repairQueued.Delete(repairWakeKey)
	}
}

func (c *Catalog) enqueuePersistedRepairs(ctx context.Context) {
	if ctx.Err() == nil {
		c.enqueueRepair(repairWakeKey)
	}
}

// drainUnknownRepairs is retained for internal callers; the due index, not a
// full unknown-row scan, now decides which paths run.
func (c *Catalog) drainUnknownRepairs(ctx context.Context, limit int) {
	if limit > 0 && ctx.Err() == nil {
		c.enqueueRepair(repairWakeKey)
	}
}

func (c *Catalog) repairLoop() {
	defer c.workers.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-c.repairCh:
			c.repairQueued.Delete(repairWakeKey)
			c.runRepairWave(c.workerCtx)
			resetRepairTimer(timer, c.nextRepairDelay(c.workerCtx))
		case <-timer.C:
			c.runRepairWave(c.workerCtx)
			resetRepairTimer(timer, c.nextRepairDelay(c.workerCtx))
		case <-c.stop:
			return
		}
	}
}

func resetRepairTimer(timer *time.Timer, delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func (c *Catalog) nextRepairDelay(ctx context.Context) time.Duration {
	if ctx.Err() != nil {
		return time.Hour
	}
	var next sql.NullInt64
	err := c.db.QueryRowContext(ctx, `SELECT MIN(repair_retry_at) FROM catalog_sessions
		WHERE turns_state='unknown' AND repair_state IN ('pending','deferred','active')`).Scan(&next)
	if err != nil || !next.Valid {
		return time.Hour
	}
	delay := time.UnixMilli(next.Int64).Sub(c.opts.Now())
	if delay < 250*time.Millisecond {
		// A failed claim must not turn an already-due row into a timer hot loop.
		// Fresh source writes still wake repairCh immediately.
		return 250 * time.Millisecond
	}
	return delay
}

func (c *Catalog) resetRepairSchedule(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, `UPDATE catalog_sessions SET
		repair_state=CASE WHEN turns_state='unknown' THEN 'pending' ELSE 'complete' END,
		repair_attempts=0,
		repair_retry_at=CASE WHEN turns_state='unknown' THEN 0 ELSE repair_retry_at END,
		repair_error_kind='', repair_engine_version=?
		WHERE repair_engine_version<>?`, repairEngineVersion, repairEngineVersion)
	return err
}

func (c *Catalog) runRepairWave(workerCtx context.Context) {
	if workerCtx.Err() != nil {
		return
	}
	started := c.opts.Now()
	processed := false
	dirty := map[string]DirectoryTarget{}
	failed := false
	for workerCtx.Err() == nil && !failed {
		items, err := c.claimDueRepairs(workerCtx, 64)
		if err != nil || len(items) == 0 {
			break
		}
		processed = true
		batch := make([]repairOutcome, 0, len(items))
		batchStarted := c.opts.Now()
		for _, item := range items {
			ctx, cancel := context.WithTimeout(workerCtx, 30*time.Second)
			var result agent.SessionListingRepairResult
			var repairErr error
			if c.testRepairSessionHook != nil {
				result, repairErr = c.testRepairSessionHook(ctx, item.path)
			} else {
				result, repairErr = agent.RepairSessionListingProjection(ctx, item.path)
			}
			cancel()
			if workerCtx.Err() != nil {
				return
			}
			batch = append(batch, repairOutcome{item: item, result: result, err: repairErr})
			if len(batch) >= 64 || c.opts.Now().Sub(batchStarted) >= 250*time.Millisecond {
				if err := c.applyRepairBatch(workerCtx, batch, dirty); err != nil {
					failed = true
					break
				}
				batch = batch[:0]
				batchStarted = c.opts.Now()
			}
			runtime.Gosched()
		}
		if !failed {
			failed = c.applyRepairBatch(workerCtx, batch, dirty) != nil
		}
	}
	for _, target := range dirty {
		c.RequestReconcile(target)
	}
	if processed {
		c.statusMu.Lock()
		c.status.LastRepairDurationMS = max(int64(0), c.opts.Now().Sub(started).Milliseconds())
		c.statusMu.Unlock()
	}
}

func (c *Catalog) claimDueRepairs(ctx context.Context, limit int) ([]repairItem, error) {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	now := c.opts.Now()
	rows, err := tx.QueryContext(ctx, `SELECT path,path_key,directory,scope,workspace_root,workspace_root_key,topic_id,repair_attempts,repair_state
		FROM catalog_sessions WHERE turns_state='unknown'
		AND repair_state IN ('pending','deferred','active') AND repair_retry_at<=?
		ORDER BY repair_retry_at ASC,last_activity_at DESC,path_key ASC LIMIT ?`, now.UnixMilli(), limit)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	var items []repairItem
	for rows.Next() {
		var item repairItem
		if err := rows.Scan(&item.path, &item.pathKey, &item.target.Path, &item.target.Scope,
			&item.target.WorkspaceRoot, &item.workspaceRootKey, &item.topicID, &item.attempts, &item.state); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for i := range items {
		lease := 30 * time.Second
		if items[i].state == "active" {
			items[i].attempts++
			lease = repairBackoff(items[i].attempts + 1)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE catalog_sessions SET repair_state='active',repair_attempts=?,
			repair_retry_at=?,repair_error_kind='' WHERE path_key=? AND turns_state='unknown'`,
			items[i].attempts, now.Add(lease).UnixMilli(), items[i].pathKey); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c.refreshCounts(ctx)
	return items, nil
}

func (c *Catalog) applyRepairBatch(ctx context.Context, outcomes []repairOutcome, dirty map[string]DirectoryTarget) error {
	if len(outcomes) == 0 || ctx.Err() != nil {
		return ctx.Err()
	}
	c.mutationMu.Lock()
	if err := c.repairBatchTestError("begin"); err != nil {
		c.mutationMu.Unlock()
		return err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		c.mutationMu.Unlock()
		return err
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		c.mutationMu.Unlock()
		return err
	}
	affected := map[TopicKey]struct{}{}
	roots := map[string]struct{}{}
	committedDirty := map[string]DirectoryTarget{}
	for _, outcome := range outcomes {
		if err := c.repairBatchTestError("update"); err != nil {
			return rollback(err)
		}
		contentFingerprint := sessionContentFingerprint(outcome.item.path)
		metaFingerprint := fileFingerprint(agent.BranchMetaPath(outcome.item.path))
		sourceFingerprint := contentFingerprint + "\x00" + metaFingerprint
		state, attempts, retryAt, errorKind, health := repairDisposition(outcome, c.opts.Now())
		if state == "complete" {
			_, err = tx.ExecContext(ctx, `UPDATE catalog_sessions SET preview=?,turns=?,turns_state='valid',health='ok',
				content_fingerprint=?,meta_fingerprint=?,repair_state='complete',repair_attempts=0,repair_retry_at=0,
				repair_error_kind='',repair_source_fingerprint=?,repair_engine_version=? WHERE path_key=?`,
				outcome.result.Preview, outcome.result.Turns, contentFingerprint, metaFingerprint,
				sourceFingerprint, repairEngineVersion, outcome.item.pathKey)
			committedDirty[queuePathKey(outcome.item.target.Path)] = outcome.item.target
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE catalog_sessions SET health=?,repair_state=?,repair_attempts=?,repair_retry_at=?,
				repair_error_kind=?,content_fingerprint=?,meta_fingerprint=?,repair_source_fingerprint=?,repair_engine_version=?
				WHERE path_key=?`, health, state, attempts, retryAt, errorKind, contentFingerprint, metaFingerprint,
				sourceFingerprint, repairEngineVersion, outcome.item.pathKey)
		}
		if err != nil {
			return rollback(err)
		}
		if outcome.item.topicID != "" {
			affected[TopicKey{Scope: outcome.item.target.Scope, WorkspaceRoot: outcome.item.target.WorkspaceRoot,
				workspaceKey: outcome.item.workspaceRootKey, TopicID: outcome.item.topicID}] = struct{}{}
		}
		roots[outcome.item.target.WorkspaceRoot] = struct{}{}
	}
	for key := range affected {
		if err := c.recomputeTopic(ctx, tx, key); err != nil {
			return rollback(err)
		}
	}
	if err := c.repairBatchTestError("revision"); err != nil {
		return rollback(err)
	}
	revision, err := bumpRevision(ctx, tx)
	if err != nil {
		return rollback(err)
	}
	if err := c.repairBatchTestError("commit"); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		c.mutationMu.Unlock()
		return err
	}
	c.mutationMu.Unlock()
	for key, target := range committedDirty {
		dirty[key] = target
	}
	c.publishRevision(revision, mapKeys(roots), "repair_batch")
	c.refreshCounts(ctx)
	return nil
}

func (c *Catalog) repairBatchTestError(stage string) error {
	if c.testRepairBatchError == nil {
		return nil
	}
	return c.testRepairBatchError(stage)
}

func repairDisposition(outcome repairOutcome, now time.Time) (state string, attempts int, retryAt int64, errorKind string, health Health) {
	if outcome.err == nil {
		switch outcome.result.Status {
		case agent.SessionListingRepairApplied, agent.SessionListingRepairAlreadyCurrent:
			return "complete", 0, 0, "", HealthOK
		case agent.SessionListingRepairDamaged:
			return "blocked", outcome.item.attempts + 1, 0, "damaged", HealthCorrupt
		case agent.SessionListingRepairUnsupported:
			return "blocked", outcome.item.attempts + 1, 0, "unsupported", HealthDegraded
		case agent.SessionListingRepairSourceChanged:
			return "deferred", outcome.item.attempts, now.Add(30 * time.Second).UnixMilli(), "source_changed", HealthDegraded
		}
	}
	attempts = outcome.item.attempts + 1
	errorKind = "io"
	if errors.Is(outcome.err, agent.ErrSessionListingRepairBusy) {
		errorKind = "busy"
	} else if errors.Is(outcome.err, context.DeadlineExceeded) {
		errorKind = "timeout"
	}
	return "deferred", attempts, now.Add(repairBackoff(attempts)).UnixMilli(), errorKind, HealthDegraded
}

func repairBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := 30 * time.Second
	for i := 1; i < attempts && delay < 30*time.Minute; i++ {
		delay *= 2
	}
	if delay > 30*time.Minute {
		return 30 * time.Minute
	}
	return delay
}

func (c *Catalog) repairSession(workerCtx context.Context, path string) {
	if workerCtx.Err() != nil {
		return
	}
	var item repairItem
	item.path = path
	item.pathKey = c.pathKey(path)
	if err := c.db.QueryRowContext(workerCtx, `SELECT directory,scope,workspace_root,workspace_root_key,topic_id,repair_attempts
		FROM catalog_sessions WHERE path_key=?`, item.pathKey).Scan(&item.target.Path, &item.target.Scope,
		&item.target.WorkspaceRoot, &item.workspaceRootKey, &item.topicID, &item.attempts); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(workerCtx, 30*time.Second)
	defer cancel()
	result, err := agent.RepairSessionListingProjection(ctx, path)
	dirty := map[string]DirectoryTarget{}
	_ = c.applyRepairBatch(workerCtx, []repairOutcome{{item: item, result: result, err: err}}, dirty)
	for _, target := range dirty {
		c.RequestReconcile(target)
	}
}

type knownSourceState struct {
	preview            string
	turns              int
	turnsState         TurnsState
	health             Health
	contentFingerprint string
}

// preserveKnownSourceStates prevents a directory scan backed by a legacy or
// transient sidecar from replacing a repaired valid/corrupt source result with
// unknown. The content fingerprint guard makes a changed transcript unknown
// again until that new generation has been parsed.
func (c *Catalog) preserveKnownSourceStates(ctx context.Context, directory string, records []SessionRecord) ([]SessionRecord, error) {
	needsKnownState := false
	for i := range records {
		if records[i].TurnsState == TurnsUnknown {
			needsKnownState = true
			break
		}
	}
	if !needsKnownState {
		return records, nil
	}
	rows, err := c.db.QueryContext(ctx, `SELECT path,preview,turns,turns_state,health,content_fingerprint
		FROM catalog_sessions WHERE directory_key=? AND missing_since=0 AND turns_state<>'unknown'`, c.pathKey(directory))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := make(map[string]knownSourceState)
	for rows.Next() {
		var path string
		var state knownSourceState
		if err := rows.Scan(&path, &state.preview, &state.turns, &state.turnsState, &state.health, &state.contentFingerprint); err != nil {
			return nil, err
		}
		known[c.pathKey(path)] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range records {
		state, ok := known[c.pathKey(records[i].Path)]
		if !ok || records[i].TurnsState != TurnsUnknown || records[i].ContentFingerprint != state.contentFingerprint {
			continue
		}
		records[i].Preview = state.preview
		records[i].Turns = state.turns
		records[i].TurnsState = state.turnsState
		records[i].Health = state.health
	}
	return records, nil
}
