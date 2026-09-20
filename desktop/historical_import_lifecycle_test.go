package main

import (
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/desktop/internal/workspacestate"
	"reasonix/internal/config"
	"reasonix/internal/identitylock"
	"reasonix/internal/session"
)

func newHistoricalLifecycleApp(t *testing.T) *App {
	t.Helper()
	app := NewApp()
	app.ctx = t.Context()
	installNoopRuntimeEvents(app)
	t.Cleanup(app.closeSessionServices)
	t.Cleanup(app.stopHistoricalImports)
	return app
}

func historicalLifecycleID(t *testing.T, app *App, title string) string {
	t.Helper()
	list, err := app.ListHistoricalSessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range list.Items {
		if view.Title == title {
			return view.ID
		}
	}
	t.Fatalf("historical source %q missing: %+v", title, list)
	return ""
}

func awaitHistoricalBatch(t *testing.T, app *App) HistoricalImportStatus {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		status, err := app.ListHistoricalSessions()
		if err != nil {
			t.Fatal(err)
		}
		if !status.Running {
			return status
		}
		select {
		case <-deadline.C:
			t.Fatalf("historical queue did not finish: %+v", status)
		case <-tick.C:
		}
	}
}

func TestHistoricalBatchContinuesPastBusySource(t *testing.T) {
	isolateDesktopUserDirs(t)
	root := config.SessionStoreDir()
	coldV4MigrationFixture(t, root, "busy")
	coldV4MigrationFixture(t, root, "available")
	app := newHistoricalLifecycleApp(t)
	busy := historicalLifecycleID(t, app, "busy")
	available := historicalLifecycleID(t, app, "available")
	release, err := identitylock.Acquire(t.Context(), filepath.Join(root, ".busy.ownership.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := app.StartHistoricalImport([]string{busy, available}); err != nil {
		t.Fatal(err)
	}
	status := awaitHistoricalBatch(t, app)
	states := map[string]string{}
	for _, view := range status.Items {
		states[view.ID] = view.Status
	}
	if states[busy] != "blocked" || states[available] != "imported" {
		t.Fatalf("one occupied source prevented independent progress: %+v", status)
	}
	if !app.runtimeRebuildMu.TryLock() {
		t.Fatal("batch retained global runtime gate")
	}
	app.runtimeRebuildMu.Unlock()
}

func TestHistoricalConcurrentRequestsKeepOneTarget(t *testing.T) {
	isolateDesktopUserDirs(t)
	coldV4MigrationFixture(t, config.SessionStoreDir(), "same-source")
	app := newHistoricalLifecycleApp(t)
	id := historicalLifecycleID(t, app, "same-source")
	entered, proceed := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(proceed) }) }
	t.Cleanup(release)
	var commits atomic.Int32
	app.desktopSessions.beforeMigrationRegistryCommit = func() error {
		if commits.Add(1) == 1 {
			close(entered)
			<-proceed
		}
		return nil
	}
	type outcome struct {
		result SessionRestoreResult
		err    error
	}
	results := make(chan outcome, 2)
	go func() { result, err := app.ImportHistoricalSession(id); results <- outcome{result, err} }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("first import did not reach publication")
	}
	started := make(chan struct{})
	go func() {
		close(started)
		result, err := app.ImportHistoricalSession(id)
		results <- outcome{result, err}
	}()
	<-started
	release()
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.result.Session != second.result.Session {
		t.Fatalf("duplicate import diverged: %+v / %+v", first, second)
	}
	if commits.Load() != 1 {
		t.Fatalf("duplicate request published %d times", commits.Load())
	}
	state, err := app.workspaceRegistry().Load(t.Context())
	if err != nil || len(state.SourceMappings) != 1 || len(state.Workspaces[workspacestate.GlobalWorkspaceID].SessionIDs) != 1 {
		t.Fatalf("duplicate durable identities: %+v %v", state, err)
	}
}

func TestHistoricalCancelCanRestartDurableImport(t *testing.T) {
	isolateDesktopUserDirs(t)
	coldV4MigrationFixture(t, config.SessionStoreDir(), "cancelled")
	app := newHistoricalLifecycleApp(t)
	id := historicalLifecycleID(t, app, "cancelled")
	entered, proceed := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(proceed) }) }
	t.Cleanup(release)
	var hooks atomic.Int32
	app.desktopSessions.beforeMigrationRegistryCommit = func() error {
		if hooks.Add(1) == 1 {
			close(entered)
			<-proceed
		}
		return nil
	}
	if _, err := app.StartHistoricalImport([]string{id}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("import did not reach publication")
	}
	if _, err := app.ControlHistoricalImport("cancel"); err != nil {
		t.Fatal(err)
	}
	release()
	awaitHistoricalBatch(t, app)
	if _, err := app.StartHistoricalImport([]string{id}); err != nil {
		t.Fatalf("cancel permanently disabled explicit import: %v", err)
	}
	status := awaitHistoricalBatch(t, app)
	if len(status.Items) != 1 || status.Items[0].Status != "imported" {
		t.Fatalf("cancelled durable import was not resumed: %+v", status)
	}
	state, err := app.workspaceRegistry().Load(t.Context())
	if err != nil || len(state.SourceMappings) != 1 || len(state.Workspaces[workspacestate.GlobalWorkspaceID].SessionIDs) != 1 {
		t.Fatalf("restart duplicated target: %+v %v", state, err)
	}
}

func TestHistoricalImportDoesNotReviveArchivedOrDeletedTarget(t *testing.T) {
	for _, lifecycle := range []string{workspacestate.Archived, workspacestate.Deleted} {
		t.Run(lifecycle, func(t *testing.T) {
			isolateDesktopUserDirs(t)
			root := config.SessionStoreDir()
			coldV4MigrationFixture(t, root, "retained-source")
			original := startupHistorySourceBytes(t, root, "retained-source")
			app := newHistoricalLifecycleApp(t)
			id := historicalLifecycleID(t, app, "retained-source")
			result, err := app.ImportHistoricalSession(id)
			if err != nil {
				t.Fatal(err)
			}
			if err := app.ArchiveCanonicalSession(result.Session); err != nil {
				t.Fatal(err)
			}
			if lifecycle == workspacestate.Deleted {
				if err := app.PurgeCanonicalSession(result.Session); err != nil {
					t.Fatal(err)
				}
			}
			retired, err := app.workspaceRegistry().Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			retainedMemberships := len(retired.Workspaces[workspacestate.GlobalWorkspaceID].SessionIDs)
			app.closeSessionServices()
			app = newHistoricalLifecycleApp(t)
			if _, err := app.ImportHistoricalSession(id); err == nil {
				t.Fatal("explicit import silently revived a retired target")
			}
			if _, err := app.StartHistoricalImport(nil); err != nil {
				t.Fatal(err)
			}
			awaitHistoricalBatch(t, app)
			state, err := app.workspaceRegistry().Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if state.SessionStates[result.Session.SessionID].Lifecycle != lifecycle || len(state.Workspaces[workspacestate.GlobalWorkspaceID].SessionIDs) != retainedMemberships || len(state.SourceMappings) != 1 {
				t.Fatalf("rescan/import revived target: %+v", state)
			}
			assertStartupHistorySourceUnchanged(t, root, "retained-source", original)
		})
	}
}

func TestHistoricalImportResumesPriorDurablePhase(t *testing.T) {
	for _, phase := range []string{"prepared", "content_ready", "content_ready_old_metadata"} {
		t.Run(phase, func(t *testing.T) {
			isolateDesktopUserDirs(t)
			root := config.SessionStoreDir()
			const sessionID = "interrupted-source"
			old := coldV4MigrationFixture(t, root, sessionID)
			app := newHistoricalLifecycleApp(t)
			workspace, err := app.ensureDesktopWorkspace(t.Context(), "global", "")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, sessionID)
			fingerprint, err := desktopSourceFingerprint(path)
			if err != nil {
				t.Fatal(err)
			}
			opID, err := app.prepareDesktopImport(t.Context(), desktopMigrationSource{scope: "global"}, path, fingerprint, sessionID, workspace)
			if err != nil {
				t.Fatal(err)
			}
			if phase != "prepared" {
				bundle := filepath.Join(t.TempDir(), "bundle")
				if err := old.Export(t.Context(), session.SessionRef{HostID: "migration-source", SessionID: sessionID}, bundle); err != nil {
					t.Fatal(err)
				}
				if _, err := app.desktopSessionService("").ImportWithHeader(t.Context(), bundle, session.CreateOptions{SessionID: sessionID, CWD: globalWorkspaceRoot(), Origin: session.SessionOriginCanonicalImport}); err != nil {
					t.Fatal(err)
				}
				if phase == "content_ready_old_metadata" {
					// Existing durable operations can omit optional presentation and
					// retained-artifact metadata; recovery must honor that snapshot.
					state, err := app.workspaceRegistry().Load(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					if err := app.workspaceRegistry().PrepareOperationContent(t.Context(), opID, []string{sessionID}, state.PendingOperations[opID].Mapping, nil); err != nil {
						t.Fatal(err)
					}
				} else {
					// Stop the real publication path immediately before CommitOperation,
					// retaining its complete provenance and presentation snapshot.
					source := desktopMigrationSource{scope: "global", operationID: opID, deferArchive: true}
					if err := app.commitDesktopImport(t.Context(), source, path, "canonical", fingerprint, sessionID, workspace); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := app.workspaceRegistry().Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			app.closeSessionServices()
			app = newHistoricalLifecycleApp(t)
			id := historicalLifecycleID(t, app, sessionID)
			result, err := app.ImportHistoricalSession(id)
			if err != nil || result.Session.SessionID != sessionID {
				t.Fatalf("durable %s import changed identity or failed: %+v %v", phase, result, err)
			}
			state, err := app.workspaceRegistry().Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if state.PendingOperations[opID].Phase != "committed" || len(state.SourceMappings) != 1 {
				t.Fatalf("prior operation was not committed exactly once: %+v", state.PendingOperations)
			}
			if phase != "prepared" {
				previous, committed := before.PendingOperations[opID], state.PendingOperations[opID]
				if !reflect.DeepEqual(previous.Mapping, committed.Mapping) || !reflect.DeepEqual(previous.Presentation, committed.Presentation) {
					t.Fatal("resuming content_ready rewrote its durable metadata")
				}
			}
			if _, err := app.desktopSessionService("").Query().Snapshot(t.Context(), result.Session); err != nil {
				t.Fatal(err)
			}
		})
	}
}
