// file_revert.go — putting one file back, without rewinding the turn.
package checkpoint

import (
	"fmt"
	"os"
	"time"

	fileenc "reasonix/internal/fileutil/encoding"
)

// PrepareFileRevert prepares a single-file restore to the earliest session preimage.
func (s *Store) PrepareFileRevert(path string, sessionRev int64) (RewindPlan, error) {
	if s == nil {
		return RewindPlan{}, fmt.Errorf("checkpoints unavailable")
	}
	plan := RewindPlan{
		PlanID:          newID("plan"),
		Scope:           RewindCode,
		Path:            path,
		SessionRevision: sessionRev,
		CreatedAt:       time.Now(),
		WorkspaceToken:  fmt.Sprintf("%d", s.barrier.Generation()),
		Files:           []string{path},
		FileCount:       1,
	}
	state, ok := s.FileState(path)
	if !ok {
		plan.CanFiles = false
		plan.DisabledReason = "file is not session-owned"
		return plan, nil
	}
	_ = state
	abs, err := safePath(s.root, path)
	if err != nil {
		plan.CanFiles = false
		plan.DisabledReason = "path unsafe"
		plan.Conflicts = []RewindConflict{{Path: path, Reason: ConflictPathUnsafe}}
		return plan, nil
	}
	revs := s.earliestRevisions(0)
	rev, has := revs[path]
	if !has {
		for p, r := range revs {
			if ap, e := safePath(s.root, p); e == nil && ap == abs {
				rev, has = r, true
				plan.Path = p
				break
			}
		}
	}
	if !has {
		plan.CanFiles = false
		plan.DisabledReason = "file is not session-owned"
		return plan, nil
	}
	if rev.AfterExisted == nil && rev.AfterSHA256 == "" {
		// A v1 or incomplete capture has a preimage but no evidence that the
		// current file is still the session's last write. Do not turn the
		// generic conflict-overwrite affordance into an unsafe legacy restore.
		plan.PlanID = ""
		plan.CanFiles = false
		plan.Legacy = true
		plan.Coverage = CoverageLegacy
		plan.DisabledReason = "legacy checkpoint cannot verify later manual edits"
		return plan, nil
	}
	fp, fperr := FingerprintPath(s.root, abs)
	if fperr == nil {
		reason := CompareIdentity(fp, rev.AfterSHA256, rev.AfterExisted, rev.AfterMode)
		if reason != "" {
			plan.Conflicts = []RewindConflict{{
				Path: path, Reason: reason,
				CheckpointSHA: rev.SHA256, LastOwnedSHA: rev.AfterSHA256, CurrentSHA: fp.SHA256,
				CurrentExisted: fp.Existed, CheckpointExist: rev.Existed,
			}}
			plan.CanFiles = true
			plan.DisabledReason = "conflict requires explicit resolution"
		} else {
			plan.CanFiles = true
		}
	} else {
		plan.CanFiles = false
		plan.DisabledReason = "current file identity unavailable"
		plan.Conflicts = []RewindConflict{{Path: path, Reason: ConflictExternalChange}}
	}
	if rev.BlobRef == "" && rev.Content == nil && rev.Existed {
		plan.CanFiles = false
		plan.DisabledReason = "missing file payload"
		plan.Conflicts = append(plan.Conflicts, RewindConflict{Path: path, Reason: ConflictMissingPayload})
	}
	s.mu.Lock()
	if s.plans == nil {
		s.plans = map[string]preparedPlan{}
	}
	s.plans[plan.PlanID] = preparedPlan{plan: plan, created: time.Now(), previewFingerprint: &fp}
	s.dropStalePlansLocked()
	s.mu.Unlock()
	return plan, nil
}

// CommitFileRevert commits a single-file restore.
func (s *Store) CommitFileRevert(planID string, resolution ConflictResolution) (RewindResult, error) {
	if s == nil {
		return RewindResult{}, fmt.Errorf("checkpoints unavailable")
	}
	s.mu.Lock()
	pp, ok := s.plans[planID]
	if ok {
		delete(s.plans, planID)
	}
	s.mu.Unlock()
	if !ok {
		return RewindResult{OK: false, Error: "unknown or expired plan"}, fmt.Errorf("unknown or expired plan")
	}
	plan := pp.plan
	if plan.Path == "" {
		return RewindResult{OK: false, Error: "not a file plan"}, fmt.Errorf("not a file plan")
	}
	if !plan.CanFiles {
		return RewindResult{OK: false, Error: plan.DisabledReason, Conflicts: plan.Conflicts}, fmt.Errorf("%s", plan.DisabledReason)
	}
	if len(plan.Conflicts) > 0 && resolution != ResolveOverwriteCheckpoint {
		if resolution == ResolveKeepCurrent {
			return RewindResult{OK: true, UndoAvailable: false}, nil
		}
		return RewindResult{OK: false, Error: "conflict requires explicit resolution", Conflicts: plan.Conflicts}, fmt.Errorf("conflict requires explicit resolution")
	}
	if !s.barrier.TryEnterExclusive() {
		err := fmt.Errorf("workspace mutation in progress")
		return RewindResult{OK: false, Error: err.Error(), Conflicts: []RewindConflict{{Path: plan.Path, Reason: ConflictBusyWriter}}}, err
	}
	defer s.barrier.ExitExclusive()
	if conflicts := s.activeWriterConflicts(); len(conflicts) > 0 {
		err := fmt.Errorf("active background writer")
		return RewindResult{OK: false, Error: err.Error(), Conflicts: conflicts}, err
	}
	if plan.WorkspaceToken != fmt.Sprintf("%d", s.barrier.Generation()) {
		conflict := RewindConflict{Path: plan.Path, Reason: ConflictStalePlan}
		return RewindResult{OK: false, Error: "workspace changed since preview", Conflicts: []RewindConflict{conflict}}, fmt.Errorf("workspace changed since preview")
	}
	absPreview, err := safePath(s.root, plan.Path)
	if err != nil {
		return RewindResult{OK: false, Error: err.Error()}, err
	}
	current, err := FingerprintPath(s.root, absPreview)
	if err != nil || pp.previewFingerprint == nil || !sameFingerprint(current, *pp.previewFingerprint) {
		conflict := RewindConflict{Path: plan.Path, Reason: ConflictStalePlan, CurrentSHA: current.SHA256, CurrentExisted: current.Existed}
		return RewindResult{OK: false, Error: "file changed since preview; preview again", Conflicts: []RewindConflict{conflict}}, fmt.Errorf("file changed since preview; preview again")
	}

	// Restore via restoreCodeLegacy for the single earliest path using turn 0.
	// Build synthetic order of one path.
	revs := s.earliestRevisions(0)
	rev, has := revs[plan.Path]
	if !has {
		abs, _ := safePath(s.root, plan.Path)
		for p, r := range revs {
			if ap, e := safePath(s.root, p); e == nil && ap == abs {
				rev, has = r, true
				plan.Path = p
				break
			}
		}
	}
	if !has {
		return RewindResult{OK: false, Error: "file is not session-owned"}, fmt.Errorf("file is not session-owned")
	}

	// Find which turn first touched this path for RestoreCode semantics:
	// restoring one file = write earliest preimage (not all files from a turn).
	abs, err := safePath(s.root, rev.Path)
	if err != nil {
		return RewindResult{OK: false, Error: err.Error()}, err
	}
	fwd, _, _ := CapturePath(abs, CaptureOptions{WorkspaceRoot: s.root, ReadContent: true})
	tx := &TransactionManifest{
		SchemaVersion: SchemaV2,
		ID:            newID("tx"),
		WorkspaceRoot: s.root,
		State:         TxPrepared,
		Kind:          "file_revert",
		Scope:         RewindCode,
		Path:          rev.Path,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	t := TransactionTarget{
		Path: rev.Path, AbsPath: abs,
		RestoreExisted: rev.Existed, RestoreMode: rev.Mode, RestoreSHA: rev.SHA256, RestoreBlob: rev.BlobRef, RestoreEncoding: rev.Encoding,
		ForwardExisted: fwd.Existed, ForwardMode: fwd.Mode, ForwardSHA: fwd.SHA256,
	}
	t.PublishTmp, t.BackupPath = transactionSiblingPaths(abs, tx.ID, 0)
	if rev.Existed {
		t.Action = "write"
		data, lerr := s.loadRevisionBytes(rev)
		if lerr != nil && rev.Content != nil {
			enc := fileenc.UTF8
			if rev.Encoding != nil {
				enc = *rev.Encoding
			} else if current := s.detectCurrentEncoding(abs); current != nil {
				enc = *current
			}
			data = fileenc.Encode(*rev.Content, enc)
			lerr = nil
		}
		if lerr != nil {
			return RewindResult{OK: false, Error: lerr.Error()}, lerr
		}
		mode := os.FileMode(0o644)
		if rev.Mode != 0 {
			mode = os.FileMode(rev.Mode)
		}
		if t.RestoreBlob == "" && s.blobs != nil {
			ref, perr := s.blobs.Put(data)
			if perr != nil {
				return RewindResult{OK: false, Error: perr.Error()}, perr
			}
			t.RestoreBlob = ref
		} else if t.RestoreBlob == "" {
			t.RestoreInline = clonePayload(data)
		}
		if err := s.writePublishTemp(t.PublishTmp, data, mode); err != nil {
			return RewindResult{OK: false, Error: err.Error()}, err
		}
	} else {
		t.Action = "delete"
		t.PublishTmp = ""
	}
	if fwd.Existed && s.blobs != nil {
		ref, perr := s.blobs.Put(fwd.Content)
		if perr != nil {
			return RewindResult{OK: false, Error: perr.Error()}, perr
		}
		t.ForwardBlob = ref
	} else if fwd.Existed {
		t.ForwardInline = clonePayload(fwd.Content)
	}
	tx.Targets = []TransactionTarget{t}
	if err := s.persistTransaction(tx); err != nil {
		return RewindResult{OK: false, Error: err.Error()}, err
	}
	return s.commitTransaction(tx, nil, nil)
}
