package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"reasonix/desktop/internal/workspacestate"
	"reasonix/internal/agent"
	"reasonix/internal/identitylock"
	"reasonix/internal/session"
	"reasonix/internal/store"
)

var errHistoricalSourceBusy = errors.New("historical session is in use; close the other instance and retry")

type HistoricalSessionView struct {
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	Format    string              `json:"format"`
	Status    string              `json:"status"`
	ErrorCode string              `json:"errorCode,omitempty"`
	Session   *session.SessionRef `json:"session,omitempty"`
}

type HistoricalImportStatus struct {
	Items     []HistoricalSessionView `json:"items"`
	Running   bool                    `json:"running"`
	Paused    bool                    `json:"paused"`
	Remaining int                     `json:"remaining"`
}

type historicalSource struct {
	path, format, scope, root, head string
}
type historicalImportCall struct {
	done   chan struct{}
	result SessionRestoreResult
	err    error
}
type historicalImportCoordinator struct {
	mu                       sync.Mutex
	sources                  map[string]historicalSource
	views                    map[string]HistoricalSessionView
	calls                    map[string]*historicalImportCall
	ctx                      context.Context
	cancel                   context.CancelFunc
	queue                    []string
	running, paused, stopped bool
	wake                     chan struct{}
	workers                  sync.WaitGroup
}

func (a *App) GetHistoricalImportStatus() HistoricalImportStatus {
	c := &a.historicalImports
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status()
}

// Listing reads directory entries and registry metadata only.
func (a *App) ListHistoricalSessions() (HistoricalImportStatus, error) {
	ctx := a.bootContext()
	state, err := a.workspaceRegistry().Load(ctx)
	if err != nil {
		return HistoricalImportStatus{Items: []HistoricalSessionView{}}, err
	}
	sources := map[string]historicalSource{}
	add := func(path, format, scope, root, head string) {
		sources[desktopSourceKey(path, head)] = historicalSource{path, format, scope, root, head}
	}
	canonical, legacy := a.desktopHistoricalRoots()
	var joined error
	for _, source := range canonical {
		joined = errors.Join(joined, scanHistoricalRoot(ctx, *source, "canonical", add))
	}
	for _, source := range legacy {
		joined = errors.Join(joined, scanHistoricalRoot(ctx, source, "legacy", add))
	}
	addHistoricalRegistrySources(state, add)
	c := &a.historicalImports
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialize(ctx)
	for id, source := range sources {
		if source.path == "" {
			continue
		}
		c.sources[id] = source
		c.views[id] = historicalImportView(state, id, source, c.views[id])
	}
	return c.status(), joined
}

func scanHistoricalRoot(ctx context.Context, source desktopMigrationSource, format string, add func(string, string, string, string, string)) error {
	entries, err := os.ReadDir(source.root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if format == "canonical" && !entry.IsDir() {
			continue
		}
		if format == "legacy" && (entry.IsDir() || !store.IsSessionTranscriptName(entry.Name())) {
			continue
		}
		path := filepath.Join(source.root, entry.Name())
		if format == "legacy" && addIndexedHistoricalHeads(path, source, add) {
			continue
		}
		add(path, format, source.scope, source.workspaceRoot, "")
	}
	return nil
}

func addIndexedHistoricalHeads(path string, source desktopMigrationSource, add func(string, string, string, string, string)) bool {
	index, err := agent.ReadSessionHeadIndex(path)
	if err != nil || index == nil || !index.Current(path) {
		return false
	}
	selected := ""
	for _, head := range index.Heads {
		if !head.Retired && head.Selected {
			selected = head.ID
		}
	}
	add(path, "legacy", source.scope, source.workspaceRoot, "")
	for _, head := range index.Heads {
		if !head.Retired && head.ID != "" && head.ID != selected {
			add(path, "legacy", source.scope, source.workspaceRoot, head.ID)
		}
	}
	return true
}

func addHistoricalRegistrySources(state workspacestate.State, add func(string, string, string, string, string)) {
	workspaceSource := func(path, format, workspaceID, head string) {
		w := state.Workspaces[workspaceID]
		scope := "project"
		if workspaceID == "global" {
			scope = "global"
		}
		add(path, format, scope, w.Root, head)
	}
	for _, mapping := range state.SourceMappings {
		workspaceSource(mapping.Path, mapping.Format, mapping.WorkspaceID, mapping.HeadID)
	}
	for _, op := range state.PendingOperations {
		if op.Mapping != nil && (op.Kind == "import" || op.Kind == "restore") {
			workspaceSource(op.Mapping.Path, op.Mapping.Format, op.WorkspaceID, op.Mapping.HeadID)
		}
	}
}

func historicalImportView(state workspacestate.State, id string, source historicalSource, view HistoricalSessionView) HistoricalSessionView {
	if view.ID == "" {
		view = HistoricalSessionView{ID: id, Title: filepath.Base(source.path), Format: source.format, Status: "available"}
	}
	mapping, ok := historicalMappingForSource(state, id)
	if !ok {
		return view
	}
	view.Status = "imported"
	ref := session.SessionRef{HostID: localDesktopHostID, SessionID: mapping.SessionID}
	view.Session = &ref
	if lifecycle := state.SessionStates[mapping.SessionID].Lifecycle; lifecycle == workspacestate.Deleted || lifecycle == workspacestate.Archived {
		view.Status = strings.ToLower(lifecycle)
		view.Session = nil
	}
	return view
}

func historicalSourceKeyMatches(mappingKey, sourceID string) bool {
	return mappingKey == sourceID || strings.HasPrefix(mappingKey, sourceID+":review:")
}

func historicalMappingForSource(state workspacestate.State, sourceID string) (workspacestate.SourceMapping, bool) {
	if mapping, ok := state.SourceMappings[sourceID]; ok {
		return mapping, true
	}
	keys := make([]string, 0, len(state.SourceMappings))
	for key := range state.SourceMappings {
		if historicalSourceKeyMatches(key, sourceID) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return workspacestate.SourceMapping{}, false
	}
	return state.SourceMappings[keys[0]], true
}

func (c *historicalImportCoordinator) initialize(ctx context.Context) {
	if c.sources != nil {
		if !c.stopped && !c.running && len(c.calls) == 0 && c.ctx.Err() != nil {
			c.ctx, c.cancel = context.WithCancel(ctx)
		}
		return
	}
	c.sources = map[string]historicalSource{}
	c.views = map[string]HistoricalSessionView{}
	c.calls = map[string]*historicalImportCall{}
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.wake = make(chan struct{}, 1)
}
func (c *historicalImportCoordinator) status() HistoricalImportStatus {
	out := HistoricalImportStatus{Items: []HistoricalSessionView{}, Running: c.running, Paused: c.paused, Remaining: len(c.queue)}
	for _, view := range c.views {
		out.Items = append(out.Items, view)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].ID < out.Items[j].ID })
	return out
}

func (a *App) ImportHistoricalSession(id string) (SessionRestoreResult, error) {
	_, listErr := a.ListHistoricalSessions()
	if listErr != nil {
		// A damaged or inaccessible historical root must not make healthy
		// sources unusable. The requested source is checked below; callers can
		// still inspect the list's per-source status for the affected root.
		c := &a.historicalImports
		c.mu.Lock()
		_, known := c.sources[id]
		c.mu.Unlock()
		if !known {
			return SessionRestoreResult{}, listErr
		}
	}
	return a.importHistoricalSession(id)
}

// Duplicate requests join one import. No runtime/controller lock is acquired
// during source access or conversion. Durable reservations protect publication.
func (a *App) importHistoricalSession(id string) (SessionRestoreResult, error) {
	c := &a.historicalImports
	c.mu.Lock()
	if c.stopped || a.shuttingDown.Load() {
		c.mu.Unlock()
		return SessionRestoreResult{}, context.Canceled
	}
	source, ok := c.sources[id]
	if !ok {
		c.mu.Unlock()
		return SessionRestoreResult{}, errors.New("historical session is unavailable; refresh the list")
	}
	ctx := c.ctx
	if call := c.calls[id]; call != nil {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.result, call.err
		case <-ctx.Done():
			return SessionRestoreResult{}, ctx.Err()
		}
	}
	call := &historicalImportCall{done: make(chan struct{})}
	c.calls[id] = call
	c.workers.Add(1)
	defer c.workers.Done()
	view := c.views[id]
	view.Status, view.ErrorCode = "importing", ""
	c.views[id] = view
	c.mu.Unlock()
	result, err := a.importHistoricalSource(ctx, id, source)
	c.mu.Lock()
	call.result, call.err = result, err
	view = c.views[id]
	if err == nil {
		view.Status = "imported"
		ref := result.Session
		view.Session = &ref
	} else {
		view.Status, view.ErrorCode = "failed", "import_failed"
		if historicalSourceBusyError(err) {
			view.Status, view.ErrorCode = "blocked", "source_busy"
			err = errHistoricalSourceBusy
		}
		if errors.Is(err, context.Canceled) {
			view.Status, view.ErrorCode = "available", "cancelled"
		}
		call.err = err
	}
	c.views[id] = view
	delete(c.calls, id)
	close(call.done)
	c.mu.Unlock()
	a.emitProjectTreeChanged()
	return result, err
}

func historicalSourceBusyError(err error) bool {
	return errors.Is(err, identitylock.ErrHeld) ||
		errors.Is(err, errHistoricalSourceBusy) ||
		errors.Is(err, agent.ErrSessionLeaseHeld) ||
		errors.Is(err, session.ErrWriterOwned)
}

func (a *App) importHistoricalSource(ctx context.Context, id string, source historicalSource) (SessionRestoreResult, error) {
	state, err := a.workspaceRegistry().Load(ctx)
	if err != nil {
		return SessionRestoreResult{}, err
	}
	if mapping, ok := historicalMappingForSource(state, id); ok {
		if state.SessionStates[mapping.SessionID].Lifecycle != workspacestate.Active {
			return SessionRestoreResult{}, errors.New("historical session was archived or deleted; use the archive to restore it")
		}
		ref := session.SessionRef{HostID: localDesktopHostID, SessionID: mapping.SessionID}
		if _, err := a.desktopSessionService("").Query().Stat(ctx, ref); err != nil {
			return SessionRestoreResult{}, err
		}
		return SessionRestoreResult{Session: ref, WorkspaceID: mapping.WorkspaceID, Generation: state.Generation}, nil
	}
	release, err := acquireHistoricalSource(ctx, id, source)
	if err != nil {
		return SessionRestoreResult{}, err
	}
	defer release()
	workspace, err := a.ensureDesktopWorkspace(ctx, source.scope, source.root)
	if err != nil {
		return SessionRestoreResult{}, err
	}
	migration := desktopMigrationSource{scope: source.scope, workspaceRoot: source.root, headID: source.head}
	// Resume the exact durable operation, including content_ready, rather than
	// creating another identity for interrupted work from previous versions.
	// The registry is a map, so choose deterministically when an older build
	// left more than one retry for the same source. A content_ready record wins
	// because its target already contains the converted projection.
	var resume *workspacestate.Operation
	for _, candidate := range state.PendingOperations {
		if candidate.Phase == "committed" || candidate.Mapping == nil || !historicalSourceKeyMatches(candidate.Mapping.SourceKey, id) {
			continue
		}
		if candidate.Kind != "import" && candidate.Kind != "restore" {
			continue
		}
		if resume == nil || historicalOperationRank(candidate) < historicalOperationRank(*resume) ||
			(historicalOperationRank(candidate) == historicalOperationRank(*resume) && candidate.ID < resume.ID) {
			copy := candidate
			resume = &copy
		}
	}
	if resume != nil {
		op := *resume
		if op.Phase == "content_ready" && op.Lifecycle == workspacestate.Active {
			if err := a.replayDesktopSessionOperation(ctx, state, op); err != nil {
				return SessionRestoreResult{}, err
			}
			updated, err := a.workspaceRegistry().Load(ctx)
			if err != nil {
				return SessionRestoreResult{}, err
			}
			committed := updated.PendingOperations[op.ID]
			return SessionRestoreResult{Session: session.SessionRef{HostID: localDesktopHostID, SessionID: committed.SessionIDs[0]}, WorkspaceID: committed.WorkspaceID, Generation: committed.ResultGeneration}, nil
		}
		migration.operationID = op.ID
	}
	err = a.convertHistoricalSource(ctx, source, migration, workspace)
	if err != nil {
		return SessionRestoreResult{}, err
	}
	state, err = a.workspaceRegistry().Load(ctx)
	if err != nil {
		return SessionRestoreResult{}, err
	}
	mapping, ok := state.SourceMappings[id]
	if !ok {
		return SessionRestoreResult{}, errors.New("historical import has not committed")
	}
	return SessionRestoreResult{Session: session.SessionRef{HostID: localDesktopHostID, SessionID: mapping.SessionID}, WorkspaceID: mapping.WorkspaceID, Generation: state.Generation}, nil
}

func historicalOperationRank(op workspacestate.Operation) int {
	switch op.Phase {
	case "content_ready":
		return 0
	case "prepared":
		return 1
	default:
		return 2
	}
}

func (a *App) convertHistoricalSource(ctx context.Context, source historicalSource, migration desktopMigrationSource, workspace string) (err error) {
	if source.format == "canonical" {
		migration.root = filepath.Dir(source.path)
		old, openErr := session.NewService("migration-source", session.NewFilesystemPersistence(migration.root))
		if openErr != nil {
			return openErr
		}
		defer func() { err = errors.Join(err, old.Shutdown(context.Background())) }()
		return a.migrateCanonicalSession(ctx, old, migration, workspace, filepath.Base(source.path))
	}
	if source.format == "legacy" || source.format == "legacy-trash" {
		return a.migrateLegacySession(ctx, source.path, migration, workspace)
	}
	return errors.New("historical format is unsupported")
}

// StartHistoricalImport snapshots the requested set; later discoveries are not
// silently added. Empty means all currently available/failed/busy sources.
func (a *App) StartHistoricalImport(ids []string) (HistoricalImportStatus, error) {
	_, listErr := a.ListHistoricalSessions()
	c := &a.historicalImports
	c.mu.Lock()
	defer c.mu.Unlock()
	if listErr != nil && len(ids) > 0 {
		for _, id := range ids {
			if _, ok := c.sources[id]; !ok {
				return c.status(), listErr
			}
		}
	}
	if c.running || c.stopped || a.shuttingDown.Load() {
		return c.status(), errors.New("historical import is already running or stopping")
	}
	if len(ids) == 0 {
		for id, v := range c.views {
			if v.Status != "imported" && v.Status != "deleted" && v.Status != "archived" {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
	}
	seen := map[string]bool{}
	queue := []string{}
	for _, id := range ids {
		if _, ok := c.sources[id]; !ok {
			return c.status(), errors.New("historical source is unavailable")
		}
		if !seen[id] {
			queue = append(queue, id)
			seen[id] = true
		}
	}
	c.queue, c.running, c.paused = queue, true, false
	go a.runHistoricalImportQueue()
	return c.status(), nil
}

func (a *App) runHistoricalImportQueue() {
	c := &a.historicalImports
	for {
		c.mu.Lock()
		if len(c.queue) == 0 || c.stopped || c.ctx.Err() != nil {
			c.running = false
			c.mu.Unlock()
			return
		}
		if c.paused {
			wake, ctx := c.wake, c.ctx
			c.mu.Unlock()
			select {
			case <-wake:
			case <-ctx.Done():
			}
			continue
		}
		id := c.queue[0]
		c.queue = c.queue[1:]
		c.mu.Unlock()
		_, _ = a.importHistoricalSession(id)
	}
}

// Pause finishes the current item. Cancel also interrupts its source work;
// durable prepared/content_ready records remain available to the next request.
func (a *App) ControlHistoricalImport(action string) (HistoricalImportStatus, error) {
	c := &a.historicalImports
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialize(a.bootContext())
	switch action {
	case "pause":
		c.paused = true
	case "resume":
		c.paused = false
		select {
		case c.wake <- struct{}{}:
		default:
		}
	case "cancel":
		c.cancel()
		c.queue = nil
		c.paused = false
	default:
		return c.status(), errors.New("unknown historical import action")
	}
	return c.status(), nil
}

func (a *App) stopHistoricalImports() {
	c := &a.historicalImports
	c.mu.Lock()
	c.initialize(a.bootContext())
	c.stopped = true
	c.queue = nil
	c.cancel()
	c.mu.Unlock()
	// Cancellation and draining happen before the runtime shutdown barrier.
	c.workers.Wait()
}

func acquireHistoricalSource(ctx context.Context, id string, source historicalSource) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lockDir := filepath.Join(desktopConfigDir(), "desktop", "historical-import-locks")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, err
	}
	release, err := identitylock.TryAcquire(filepath.Join(lockDir, id+".lock"))
	if err != nil {
		return nil, err
	}
	if source.format != "canonical" {
		return release, nil
	}
	ownership, err := identitylock.TryAcquireMode(filepath.Join(filepath.Dir(source.path), "."+filepath.Base(source.path)+".ownership.lock"), identitylock.ModeShared)
	if err != nil {
		release()
		return nil, err
	}
	return func() { ownership(); release() }, nil
}

// Legacy recovery RPCs share cancellation/draining with the on-demand queue.
func (a *App) beginHistoricalRecovery() (context.Context, func(), error) {
	c := &a.historicalImports
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialize(a.bootContext())
	if c.stopped || a.shuttingDown.Load() {
		return nil, nil, context.Canceled
	}
	c.workers.Add(1)
	return c.ctx, c.workers.Done, nil
}
