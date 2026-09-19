package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"reasonix/desktop/internal/draftstate"
	"reasonix/desktop/internal/legacycleanup"
	"reasonix/desktop/internal/workspacestate"
	"reasonix/internal/agent"
	"reasonix/internal/session"
	"reasonix/internal/store"
)

type savedTabReconcileOutcome string

const (
	restoreTab            savedTabReconcileOutcome = "restore"
	dropStalePresentation savedTabReconcileOutcome = "drop_stale_presentation"
	archiveEmptyThenDrop  savedTabReconcileOutcome = "archive_empty_then_drop"
	preserveRecovery      savedTabReconcileOutcome = "preserve_recovery"
	preserveError         savedTabReconcileOutcome = "preserve_error"
)

type savedTabReconcileDecision struct {
	outcome            savedTabReconcileOutcome
	reason             string
	identityKind       string
	waitedForMigration bool
	hadPending         bool
	hadRecoveryOwner   bool
	repairedIdentity   bool
}

type savedTabReconcileEvidence struct {
	registry    workspacestate.State
	registryErr error
	draftOps    []draftstate.Operation
	draftErr    error
	cleanup     legacycleanup.State
}

func (a *App) reconcileTabsBeforeRestore(ctx context.Context, file desktopTabsFile, version uint64) (desktopTabsFile, uint64, bool) {
	original := file
	reconciled, changed := a.reconcileSavedTabs(ctx, file)
	if !a.tabsSnapshotCurrent(version) {
		return file, version, false
	}
	if changed {
		committedVersion, err := a.persistReconciledTabsFile(reconciled, version)
		if committedVersion != 0 {
			version = committedVersion
		}
		if errors.Is(err, errTabsSnapshotChanged) {
			return file, version, false
		}
		if err != nil {
			slog.Warn("desktop_saved_tab_reconcile_persist_failed", "reason", "write_failed")
			return blockSavedTabUnsafeRestore(original, "identity_repair_write_failed"), version, a.tabsSnapshotCurrent(version)
		}
	}
	return reconciled, version, a.tabsSnapshotCurrent(version)
}

// reconcileSavedTabs filters only presentation entries whose durable identity
// is conclusively gone. It runs before restored tabs are published, so a stale
// entry can neither block legacy cleanup nor acquire a controller or lease.
func (a *App) reconcileSavedTabs(ctx context.Context, file desktopTabsFile) (desktopTabsFile, bool) {
	file.Tabs = append([]desktopTabEntry(nil), file.Tabs...)
	if len(file.Tabs) == 0 {
		file.Tabs = []desktopTabEntry{}
		return file, false
	}

	fast := a.loadSavedTabReconcileEvidence(ctx, false)
	decisions := make([]savedTabReconcileDecision, len(file.Tabs))
	needsMigration := make([]bool, len(file.Tabs))
	anyNeedsMigration := false
	repairedIdentity := false
	for index := range file.Tabs {
		if candidate := savedTabRouteCandidateForPath(file.Tabs[index].SessionPath); candidate.kind != "" {
			decisions[index].identityKind = candidate.kind
			needsMigration[index] = true
			anyNeedsMigration = true
			continue
		}
		if sessionID, found, conflict := savedTabPendingSessionIdentity(file.Tabs[index], fast); found && !conflict {
			file.Tabs[index].SessionID = sessionID
			repairedIdentity = true
		}
		entry := file.Tabs[index]
		decision, final := a.classifySavedTab(entry, fast, false)
		decisions[index] = decision
		needsMigration[index] = !final
		anyNeedsMigration = anyNeedsMigration || !final
	}

	if anyNeedsMigration {
		migrationFinished := a.waitForDesktopMigration(ctx)
		afterMigration := a.loadSavedTabReconcileEvidence(ctx, true)
		for index := range file.Tabs {
			if !needsMigration[index] {
				continue
			}
			identityKind := decisions[index].identityKind
			if !migrationFinished {
				decisions[index] = savedTabReconcileDecision{outcome: preserveError, reason: "migration_interrupted", identityKind: identityKind, waitedForMigration: true}
				continue
			}
			if identityKind != "" {
				normalized, repaired, override := a.normalizeSavedTabRoute(ctx, file.Tabs[index], afterMigration)
				if override != nil {
					override.identityKind = identityKind
					override.waitedForMigration = true
					decisions[index] = *override
					continue
				}
				file.Tabs[index] = normalized
				if repaired {
					repairedIdentity = true
				}
			}
			if sessionID, found, conflict := savedTabPendingSessionIdentity(file.Tabs[index], afterMigration); found && !conflict {
				file.Tabs[index].SessionID = sessionID
				repairedIdentity = true
			}
			decision, _ := a.classifySavedTab(file.Tabs[index], afterMigration, true)
			if identityKind != "" {
				decision.identityKind = identityKind
			}
			decision.repairedIdentity = strings.TrimSpace(file.Tabs[index].SessionID) != "" && strings.TrimSpace(file.Tabs[index].SessionPath) == "" && identityKind != ""
			decision.waitedForMigration = true
			decisions[index] = decision
		}
	}

	filtered := make([]desktopTabEntry, 0, len(file.Tabs))
	removed := map[string]bool{}
	for index, entry := range file.Tabs {
		decision := decisions[index]
		if decision.outcome == dropStalePresentation || decision.outcome == archiveEmptyThenDrop {
			removed[entry.ID] = true
		} else {
			if decision.identityKind != "" && (decision.outcome == preserveError || decision.outcome == preserveRecovery) {
				entry.restoreBlocked = true
				entry.restoreBlockReason = decision.reason
			}
			filtered = append(filtered, entry)
		}
		if decision.outcome != restoreTab || decision.waitedForMigration || decision.repairedIdentity {
			identityKind := decision.identityKind
			if identityKind == "" {
				identityKind = savedTabIdentityKind(entry)
			}
			slog.Info("desktop_saved_tab_reconciled",
				"outcome", decision.outcome,
				"reason", decision.reason,
				"identity_kind", identityKind,
				"waited_for_migration", decision.waitedForMigration,
				"had_pending_operation", decision.hadPending,
				"had_recovery_owner", decision.hadRecoveryOwner,
			)
		}
	}
	file.Tabs = filtered
	if len(removed) == 0 && !repairedIdentity {
		return file, false
	}
	repairReconciledTabSelection(&file, removed)
	return file, true
}

func (a *App) waitForDesktopMigration(ctx context.Context) bool {
	if a == nil || a.desktopMigrationDone == nil {
		return false
	}
	if a.beforeSavedTabMigrationWait != nil {
		a.beforeSavedTabMigrationWait()
	}
	select {
	case <-a.desktopMigrationDone:
		return !a.desktopMigrationFailed.Load()
	case <-ctx.Done():
		return false
	}
}

func (a *App) loadSavedTabReconcileEvidence(ctx context.Context, includeCleanup bool) savedTabReconcileEvidence {
	evidence := savedTabReconcileEvidence{}
	evidence.registry, evidence.registryErr = a.workspaceRegistry().Load(ctx)
	evidence.draftOps, evidence.draftErr = a.draftStore().PendingOperations(ctx)
	if includeCleanup && a.legacyCleanup != nil {
		evidence.cleanup, _ = a.legacyCleanup.Load(ctx)
	}
	return evidence
}

func (a *App) classifySavedTab(entry desktopTabEntry, evidence savedTabReconcileEvidence, afterMigration bool) (savedTabReconcileDecision, bool) {
	if !afterMigration {
		if evidence.registryErr != nil || evidence.draftErr != nil {
			return savedTabReconcileDecision{}, false
		}
		if savedTabHasMatchingPendingCreate(entry, evidence) {
			return savedTabReconcileDecision{outcome: restoreTab, reason: "pending_create", hadPending: true}, true
		}
	}

	if sessionID := strings.TrimSpace(entry.SessionID); sessionID != "" {
		return a.classifyCanonicalSavedTab(entry, sessionID, evidence, afterMigration)
	}
	if strings.TrimSpace(entry.SessionPath) != "" {
		if !afterMigration {
			return savedTabReconcileDecision{}, false
		}
		if _, ok, err := legacySessionPathForFileAccess(entry.SessionPath); err != nil || !ok {
			return savedTabReconcileDecision{outcome: preserveError, reason: "invalid_legacy_path", identityKind: "invalid_legacy"}, true
		}
		return a.classifyLegacySavedTab(entry, evidence), true
	}
	if strings.TrimSpace(entry.CreateOperationID) != "" && !afterMigration {
		return savedTabReconcileDecision{}, false
	}
	if strings.TrimSpace(entry.CreateOperationID) != "" {
		return savedTabReconcileDecision{outcome: preserveError, reason: "pending_identity_unresolved", hadPending: true}, true
	}
	if afterMigration && (evidence.registryErr != nil || evidence.draftErr != nil) {
		return savedTabReconcileDecision{outcome: preserveError, reason: "persistence_state_unavailable"}, true
	}
	return savedTabReconcileDecision{outcome: dropStalePresentation, reason: "identity_absent"}, true
}

func (a *App) classifyCanonicalSavedTab(entry desktopTabEntry, sessionID string, evidence savedTabReconcileEvidence, afterMigration bool) (savedTabReconcileDecision, bool) {
	if evidence.registryErr != nil || evidence.draftErr != nil {
		if afterMigration {
			return savedTabReconcileDecision{outcome: preserveError, reason: "persistence_state_unavailable"}, true
		}
		return savedTabReconcileDecision{}, false
	}
	if savedTabHasMatchingPendingCreate(entry, evidence) {
		return savedTabReconcileDecision{outcome: restoreTab, reason: "pending_create", hadPending: true}, true
	}

	status, registered := evidence.registry.SessionStates[sessionID]
	if registered && (status.Lifecycle == workspacestate.Archived || status.Lifecycle == workspacestate.Deleted) {
		return savedTabReconcileDecision{outcome: dropStalePresentation, reason: "inactive_lifecycle"}, true
	}
	info, statErr := a.desktopSessionService("").Query().Stat(a.bootContext(), session.SessionRef{HostID: localDesktopHostID, SessionID: sessionID})
	if !afterMigration {
		if registered && status.Lifecycle == workspacestate.Active && statErr == nil {
			workspace, consistent := savedTabCanonicalWorkspace(evidence.registry, info, sessionID)
			if consistent && savedTabMatchesWorkspace(entry, workspace) {
				return savedTabReconcileDecision{outcome: restoreTab, reason: "canonical_active"}, true
			}
		}
		return savedTabReconcileDecision{}, false
	}

	recoveryOwner := savedTabHasRecoveryOwner(entry, evidence)
	if registered && status.Lifecycle == workspacestate.Active {
		if errors.Is(statErr, session.ErrSessionNotFound) || errors.Is(statErr, os.ErrNotExist) {
			return savedTabReconcileDecision{outcome: preserveRecovery, reason: "registered_session_missing", hadRecoveryOwner: true}, true
		}
		if statErr != nil || info.MetadataStatus == session.MetadataFailed {
			return savedTabReconcileDecision{outcome: preserveError, reason: "canonical_session_unreadable", hadRecoveryOwner: recoveryOwner}, true
		}
		workspace, consistent := savedTabCanonicalWorkspace(evidence.registry, info, sessionID)
		if !consistent {
			if recoveryOwner {
				return savedTabReconcileDecision{outcome: preserveRecovery, reason: "recovery_owner_present", hadRecoveryOwner: true}, true
			}
			if a.archiveSavedTabEmptyCandidate(entry, evidence.cleanup) {
				return savedTabReconcileDecision{outcome: archiveEmptyThenDrop, reason: "empty_workspace_conflict"}, true
			}
			return savedTabReconcileDecision{outcome: preserveError, reason: "canonical_workspace_conflict", hadRecoveryOwner: recoveryOwner}, true
		}
		if !savedTabMatchesWorkspace(entry, workspace) {
			return savedTabReconcileDecision{outcome: restoreTab, reason: "repair_workspace"}, true
		}
		return savedTabReconcileDecision{outcome: restoreTab, reason: "canonical_active"}, true
	}
	if statErr == nil {
		return savedTabReconcileDecision{outcome: preserveRecovery, reason: "canonical_session_unregistered", hadRecoveryOwner: true}, true
	}
	if !errors.Is(statErr, session.ErrSessionNotFound) && !errors.Is(statErr, os.ErrNotExist) {
		return savedTabReconcileDecision{outcome: preserveError, reason: "canonical_session_unreadable", hadRecoveryOwner: recoveryOwner}, true
	}
	if recoveryOwner {
		return savedTabReconcileDecision{outcome: preserveRecovery, reason: "recovery_owner_present", hadRecoveryOwner: true}, true
	}
	if present, err := a.savedTabCanonicalArtifactsPresent(entry, sessionID); err != nil {
		return savedTabReconcileDecision{outcome: preserveError, reason: "canonical_artifacts_unreadable"}, true
	} else if present {
		return savedTabReconcileDecision{outcome: preserveRecovery, reason: "canonical_artifacts_present", hadRecoveryOwner: true}, true
	}
	return savedTabReconcileDecision{outcome: dropStalePresentation, reason: "canonical_identity_absent"}, true
}

func (a *App) classifyLegacySavedTab(entry desktopTabEntry, evidence savedTabReconcileEvidence) savedTabReconcileDecision {
	if evidence.registryErr != nil || evidence.draftErr != nil {
		return savedTabReconcileDecision{outcome: preserveError, reason: "persistence_state_unavailable"}
	}
	path := strings.TrimSpace(entry.SessionPath)
	mapping, mapped, mappingErr := selectedSavedTabSourceMapping(path, evidence.registry)
	if mappingErr != nil {
		return savedTabReconcileDecision{outcome: preserveError, reason: "legacy_mapping_unreadable"}
	}
	if mapped {
		switch evidence.registry.SessionStates[mapping.SessionID].Lifecycle {
		case workspacestate.Active:
			return savedTabReconcileDecision{outcome: restoreTab, reason: "legacy_source_mapped", hadRecoveryOwner: true}
		case workspacestate.Archived, workspacestate.Deleted:
			fingerprint, err := desktopSourceFingerprint(path)
			if errors.Is(err, os.ErrNotExist) {
				if _, artifactErr := legacyCleanupSourceFingerprint(path); artifactErr == nil {
					return savedTabReconcileDecision{outcome: preserveRecovery, reason: "legacy_artifacts_present", hadRecoveryOwner: true}
				} else if !errors.Is(artifactErr, os.ErrNotExist) {
					return savedTabReconcileDecision{outcome: preserveError, reason: "legacy_artifacts_unreadable"}
				}
				return savedTabReconcileDecision{outcome: dropStalePresentation, reason: "mapped_session_inactive"}
			}
			if err != nil {
				return savedTabReconcileDecision{outcome: preserveError, reason: "legacy_artifacts_unreadable"}
			}
			if mapping.Fingerprint != "" && fingerprint == mapping.Fingerprint {
				return savedTabReconcileDecision{outcome: dropStalePresentation, reason: "mapped_session_inactive"}
			}
			return savedTabReconcileDecision{outcome: preserveRecovery, reason: "legacy_source_changed", hadRecoveryOwner: true}
		default:
			return savedTabReconcileDecision{outcome: preserveRecovery, reason: "source_mapping_incomplete", hadRecoveryOwner: true}
		}
	}
	if savedTabHasRecoveryOwner(entry, evidence) {
		return savedTabReconcileDecision{outcome: preserveRecovery, reason: "recovery_owner_present", hadRecoveryOwner: true}
	}
	if _, err := legacyCleanupSourceFingerprint(path); err == nil {
		return savedTabReconcileDecision{outcome: preserveRecovery, reason: "legacy_artifacts_present", hadRecoveryOwner: true}
	} else if !errors.Is(err, os.ErrNotExist) {
		return savedTabReconcileDecision{outcome: preserveError, reason: "legacy_artifacts_unreadable"}
	}
	return savedTabReconcileDecision{outcome: dropStalePresentation, reason: "legacy_identity_absent"}
}

func (a *App) savedTabCanonicalArtifactsPresent(entry desktopTabEntry, sessionID string) (bool, error) {
	canonicalPath := filepath.Join(a.desktopSessions.root, sessionID)
	if _, err := os.Lstat(canonicalPath); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	legacyPath := strings.TrimSpace(entry.SessionPath)
	if legacyPath == "" {
		return false, nil
	}
	if _, err := legacyCleanupSourceFingerprint(legacyPath); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, nil
}

func selectedSavedTabSourceMapping(path string, state workspacestate.State) (workspacestate.SourceMapping, bool, error) {
	for _, mapping := range state.SourceMappings {
		if mapping.HeadID == "" && sessionRuntimeKey(mapping.Path) == sessionRuntimeKey(path) {
			return mapping, true, nil
		}
	}
	hasHeadMapping := false
	for _, mapping := range state.SourceMappings {
		if mapping.HeadID != "" && sessionRuntimeKey(mapping.Path) == sessionRuntimeKey(path) {
			hasHeadMapping = true
			break
		}
	}
	if !hasHeadMapping {
		return workspacestate.SourceMapping{}, false, nil
	}
	heads, err := agent.ListSessionHeads(path)
	if err != nil {
		return workspacestate.SourceMapping{}, false, err
	}
	for _, head := range heads {
		if !head.Selected || head.Retired {
			continue
		}
		for _, mapping := range state.SourceMappings {
			if mapping.HeadID == head.ID && sessionRuntimeKey(mapping.Path) == sessionRuntimeKey(path) {
				return mapping, true, nil
			}
		}
		return workspacestate.SourceMapping{}, false, nil
	}
	return workspacestate.SourceMapping{}, false, nil
}

func savedTabHasMatchingPendingCreate(entry desktopTabEntry, evidence savedTabReconcileEvidence) bool {
	operationID := strings.TrimSpace(entry.CreateOperationID)
	sessionID := strings.TrimSpace(entry.SessionID)
	if operationID == "" || sessionID == "" {
		return false
	}
	if pending, ok := evidence.registry.PendingCreates[sessionID]; ok && pending.OperationID == operationID {
		return true
	}
	for _, operation := range evidence.draftOps {
		if operation.ID == operationID && operation.SessionID == sessionID {
			return true
		}
	}
	return false
}

func savedTabPendingSessionIdentity(entry desktopTabEntry, evidence savedTabReconcileEvidence) (string, bool, bool) {
	if strings.TrimSpace(entry.SessionID) != "" {
		return "", false, false
	}
	operationID := strings.TrimSpace(entry.CreateOperationID)
	if operationID == "" || evidence.registryErr != nil || evidence.draftErr != nil {
		return "", false, false
	}
	resolved := ""
	accept := func(sessionID string) bool {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return true
		}
		if resolved != "" && resolved != sessionID {
			return false
		}
		resolved = sessionID
		return true
	}
	for sessionID, pending := range evidence.registry.PendingCreates {
		if pending.OperationID == operationID && !accept(sessionID) {
			return "", false, true
		}
	}
	for _, operation := range evidence.draftOps {
		if operation.ID == operationID && !accept(operation.SessionID) {
			return "", false, true
		}
	}
	return resolved, resolved != "", false
}

func savedTabHasRecoveryOwner(entry desktopTabEntry, evidence savedTabReconcileEvidence) bool {
	sessionID := strings.TrimSpace(entry.SessionID)
	pathKey := sessionRuntimeKey(entry.SessionPath)
	if pending, ok := evidence.registry.PendingCreates[sessionID]; sessionID != "" && ok && pending.SessionID == sessionID {
		return true
	}
	for _, operation := range evidence.draftOps {
		if sessionID != "" && operation.SessionID == sessionID {
			return true
		}
	}
	for _, operation := range evidence.registry.PendingOperations {
		if sessionID != "" && slices.Contains(operation.SessionIDs, sessionID) {
			return true
		}
		if operation.Mapping != nil && ((sessionID != "" && operation.Mapping.SessionID == sessionID) || (pathKey != "" && sessionRuntimeKey(operation.Mapping.Path) == pathKey)) {
			return true
		}
	}
	for _, mapping := range evidence.registry.SourceMappings {
		if (sessionID != "" && mapping.SessionID == sessionID) || (pathKey != "" && sessionRuntimeKey(mapping.Path) == pathKey) {
			return true
		}
	}
	for _, recovery := range evidence.registry.RecoveryEntries {
		if (sessionID != "" && recovery.SessionID == sessionID) || (pathKey != "" && sessionRuntimeKey(recovery.Path) == pathKey) {
			return true
		}
	}
	return false
}

func savedTabCanonicalWorkspace(state workspacestate.State, info session.SessionInfo, sessionID string) (workspacestate.Workspace, bool) {
	var owner workspacestate.Workspace
	for _, workspace := range state.Workspaces {
		if !slices.Contains(workspace.SessionIDs, sessionID) {
			continue
		}
		if owner.ID != "" {
			return workspacestate.Workspace{}, false
		}
		owner = workspace
	}
	if owner.ID == "" || info.Origin == "" || strings.TrimSpace(info.CWD) == "" || !sameDesktopPath(info.CWD, owner.Root) {
		return workspacestate.Workspace{}, false
	}
	return owner, true
}

func savedTabMatchesWorkspace(entry desktopTabEntry, workspace workspacestate.Workspace) bool {
	return restoredWorkspaceID(entry) == workspace.ID && sameDesktopPath(desktopWorkspaceRoot(entry.Scope, entry.WorkspaceRoot), workspace.Root)
}

func (a *App) archiveSavedTabEmptyCandidate(entry desktopTabEntry, cleanup legacycleanup.State) bool {
	var candidate legacycleanup.Candidate
	for _, item := range cleanup.Items {
		if entry.SessionID != "" && item.SessionID == entry.SessionID {
			candidate = item
			break
		}
	}
	if candidate.ID == "" && entry.SessionID == "" {
		for _, item := range cleanup.Items {
			if entry.SessionPath != "" && sameDesktopPath(item.SourcePath, entry.SessionPath) {
				candidate = item
				break
			}
		}
	}
	if candidate.ID == "" {
		return false
	}
	if candidate.Restored || candidate.Phase == "archived" || candidate.Phase == "has_content" || candidate.Phase == "protected" {
		return false
	}
	switch candidate.Kind {
	case "session":
		a.processLegacyCleanupSession(candidate)
	case "legacy":
		a.processLegacyCleanupSource(candidate)
	default:
		return false
	}
	state, err := a.workspaceRegistry().Load(a.bootContext())
	if err != nil {
		return false
	}
	sessionID := strings.TrimSpace(entry.SessionID)
	if sessionID == "" {
		if refreshed, loadErr := a.legacyCleanup.Load(a.bootContext()); loadErr == nil {
			sessionID = refreshed.Items[candidate.ID].SessionID
		}
	}
	return sessionID != "" && state.SessionStates[sessionID].Lifecycle == workspacestate.Archived
}

func repairReconciledTabSelection(file *desktopTabsFile, removed map[string]bool) {
	local := make(map[string]bool, len(file.Tabs))
	remote := make(map[string]bool, len(file.RemoteTabs))
	for _, entry := range file.Tabs {
		local[entry.ID] = true
	}
	for _, entry := range file.RemoteTabs {
		remote[entry.ID] = true
	}
	order := make([]string, 0, len(file.TabOrder))
	for _, id := range file.TabOrder {
		if !removed[id] && (local[id] || remote[id]) {
			order = append(order, id)
		}
	}
	file.TabOrder = order
	if local[file.ActiveTab] || remote[file.ActiveTab] {
		return
	}
	file.ActiveTab = ""
	if len(order) > 0 {
		file.ActiveTab = order[0]
	} else if len(file.Tabs) > 0 {
		file.ActiveTab = file.Tabs[0].ID
	} else if len(file.RemoteTabs) > 0 {
		file.ActiveTab = file.RemoteTabs[0].ID
	}
}

func savedTabIdentityKind(entry desktopTabEntry) string {
	if strings.TrimSpace(entry.SessionID) != "" {
		return "canonical"
	}
	if strings.TrimSpace(entry.SessionPath) != "" {
		return "legacy"
	}
	return "none"
}

type savedTabRouteCandidate struct {
	sessionID string
	kind      string
}

func savedTabRouteCandidateForPath(raw string) savedTabRouteCandidate {
	value := strings.TrimSpace(raw)
	if value == "" {
		return savedTabRouteCandidate{}
	}
	if strings.HasPrefix(value, remoteSessionIDRoutePrefix) {
		locator := classifySessionLocator(value)
		if locator.kind != sessionLocatorCanonical {
			return savedTabRouteCandidate{kind: "invalid_route"}
		}
		return savedTabRouteCandidate{sessionID: locator.ref.SessionID, kind: "canonical_route"}
	}
	if !persistedPathLooksAbsolute(value) {
		return savedTabRouteCandidate{}
	}
	normalized := strings.ReplaceAll(value, `\`, "/")
	base := normalized[strings.LastIndex(normalized, "/")+1:]
	if !strings.HasPrefix(base, remoteSessionIDRoutePrefix) {
		return savedTabRouteCandidate{}
	}
	// A real legacy transcript or sidecar can legally contain a colon on
	// POSIX. Those names remain paths and are never upgraded by this repair.
	if strings.HasSuffix(strings.ToLower(base), ".jsonl") || strings.HasSuffix(strings.ToLower(base), ".meta") {
		return savedTabRouteCandidate{}
	}
	locator := classifySessionLocator(base)
	if locator.kind != sessionLocatorCanonical {
		return savedTabRouteCandidate{kind: "invalid_route"}
	}
	return savedTabRouteCandidate{sessionID: locator.ref.SessionID, kind: "pseudo_route_path"}
}

func persistedPathLooksAbsolute(path string) bool {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return true
	}
	return len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func (a *App) normalizeSavedTabRoute(ctx context.Context, entry desktopTabEntry, evidence savedTabReconcileEvidence) (desktopTabEntry, bool, *savedTabReconcileDecision) {
	candidate := savedTabRouteCandidateForPath(entry.SessionPath)
	failed := func(reason string, pending, recovery bool) (desktopTabEntry, bool, *savedTabReconcileDecision) {
		return entry, false, &savedTabReconcileDecision{
			outcome: preserveError, reason: reason, identityKind: candidate.kind,
			hadPending: pending, hadRecoveryOwner: recovery,
		}
	}
	if candidate.kind == "" {
		return entry, false, nil
	}
	if candidate.kind == "invalid_route" || candidate.sessionID == "" {
		return failed("invalid_route", false, false)
	}
	if id := strings.TrimSpace(entry.SessionID); id != "" && id != candidate.sessionID {
		return failed("identity_conflict", false, false)
	}

	pendingID, pendingFound, pendingConflict := savedTabPendingSessionIdentity(desktopTabEntry{
		CreateOperationID: entry.CreateOperationID,
	}, evidence)
	if pendingConflict || (pendingFound && pendingID != candidate.sessionID) {
		return failed("pending_identity_conflict", true, false)
	}
	if candidate.kind == "canonical_route" {
		entry.SessionID = candidate.sessionID
		entry.SessionPath = ""
		return entry, true, nil
	}

	present, artifactErr := savedTabPseudoRouteArtifactsPresent(entry.SessionPath)
	if artifactErr != nil {
		return failed("legacy_artifacts_unreadable", pendingFound, false)
	}
	if present {
		return failed("legacy_artifacts_present", pendingFound, false)
	}
	confirmed, conflict, recoveryOwner := a.savedTabPseudoRouteIdentityConfirmed(ctx, entry, candidate.sessionID, evidence, pendingID, pendingFound)
	if conflict {
		return failed("identity_conflict", pendingFound, recoveryOwner)
	}
	if !confirmed {
		return failed("identity_evidence_absent", pendingFound, recoveryOwner)
	}
	entry.SessionID = candidate.sessionID
	entry.SessionPath = ""
	return entry, true, nil
}

func savedTabPseudoRouteArtifactsPresent(path string) (bool, error) {
	return savedTabPseudoRouteArtifactsPresentWith(path, os.Lstat)
}

func savedTabPseudoRouteArtifactsPresentWith(path string, lstat func(string) (os.FileInfo, error)) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, nil
	}
	if runtime.GOOS == "windows" && savedTabRouteCandidateForPath(path).kind == "pseudo_route_path" {
		// The route colon is illegal in a Windows file name, so this spelling
		// cannot identify a real transcript or sidecar on the native filesystem.
		return false, nil
	}
	// Foreign Windows paths on POSIX cannot name a local artifact. Native
	// paths are probed without cleaning so the persisted spelling is preserved.
	if runtime.GOOS != "windows" && !filepath.IsAbs(path) {
		return false, nil
	}
	artifacts := append([]string{path}, store.SessionSidecarFiles(path)...)
	artifacts = append(artifacts,
		store.SessionLockFile(path), store.SessionLeaseLock(path), store.SessionLeaseInfo(path),
		store.SessionCheckpointDir(path), store.SessionJobsDir(path), store.SessionInboxDir(path),
		store.SessionCleanupPending(path),
	)
	seen := make(map[string]bool, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == "" || seen[artifact] {
			continue
		}
		seen[artifact] = true
		if _, err := lstat(artifact); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func (a *App) savedTabPseudoRouteIdentityConfirmed(ctx context.Context, entry desktopTabEntry, sessionID string, evidence savedTabReconcileEvidence, pendingID string, pendingFound bool) (bool, bool, bool) {
	confirmed := pendingFound && pendingID == sessionID
	owners := savedTabDurableRouteOwnerIDs(entry, sessionID, evidence)
	if len(owners) > 1 || (len(owners) == 1 && !owners[sessionID]) {
		return false, true, len(owners) > 0
	}
	if owners[sessionID] {
		confirmed = true
	}

	info, err := a.desktopSessionService("").Query().Stat(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: sessionID})
	if err == nil {
		workspace, consistent := savedTabCanonicalWorkspace(evidence.registry, info, sessionID)
		if consistent && savedTabMatchesWorkspace(entry, workspace) {
			confirmed = true
		}
	}
	return confirmed, false, len(owners) > 0
}

func savedTabDurableRouteOwnerIDs(entry desktopTabEntry, candidateID string, evidence savedTabReconcileEvidence) map[string]bool {
	owners := map[string]bool{}
	operationID := strings.TrimSpace(entry.CreateOperationID)
	rawPath := strings.TrimSpace(entry.SessionPath)
	workspaceID := restoredWorkspaceID(entry)
	for sessionID, pending := range evidence.registry.PendingCreates {
		if (operationID != "" && pending.OperationID == operationID) || (sessionID == candidateID && pending.WorkspaceID == workspaceID) {
			owners[strings.TrimSpace(sessionID)] = true
		}
	}
	for _, operation := range evidence.draftOps {
		if operationID != "" && operation.ID == operationID {
			owners[strings.TrimSpace(operation.SessionID)] = true
		}
	}
	for _, operation := range evidence.registry.PendingOperations {
		if operationID != "" && strings.TrimSpace(operation.ID) == operationID {
			for _, id := range operation.SessionIDs {
				owners[strings.TrimSpace(id)] = true
			}
		}
		if operation.Mapping != nil && strings.TrimSpace(operation.Mapping.Path) == rawPath {
			owners[strings.TrimSpace(operation.Mapping.SessionID)] = true
		}
		if operation.WorkspaceID == workspaceID && slices.Contains(operation.SessionIDs, candidateID) {
			owners[candidateID] = true
		}
	}
	for _, mapping := range evidence.registry.SourceMappings {
		if strings.TrimSpace(mapping.Path) == rawPath {
			owners[strings.TrimSpace(mapping.SessionID)] = true
		}
		if mapping.WorkspaceID == workspaceID && mapping.SessionID == candidateID {
			owners[candidateID] = true
		}
	}
	for _, recovery := range evidence.registry.RecoveryEntries {
		if strings.TrimSpace(recovery.Path) == rawPath {
			owners[strings.TrimSpace(recovery.SessionID)] = true
		}
		if recovery.SessionID == candidateID && (recovery.WorkspaceID == workspaceID || recovery.Scope == entry.Scope && sameDesktopPath(recovery.WorkspaceRoot, entry.WorkspaceRoot)) {
			owners[candidateID] = true
		}
	}
	delete(owners, "")
	return owners
}

func blockSavedTabUnsafeRestore(file desktopTabsFile, reason string) desktopTabsFile {
	file.Tabs = append([]desktopTabEntry(nil), file.Tabs...)
	for index := range file.Tabs {
		path := strings.TrimSpace(file.Tabs[index].SessionPath)
		unsafe := savedTabRouteCandidateForPath(path).kind != ""
		if !unsafe && path != "" {
			_, ok, err := legacySessionPathForFileAccess(path)
			unsafe = err != nil || !ok
		}
		if unsafe {
			file.Tabs[index].restoreBlocked = true
			file.Tabs[index].restoreBlockReason = reason
		}
	}
	return file
}
