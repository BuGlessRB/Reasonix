package agent

import (
	"errors"
	"time"
)

func (s *SubagentStore) SaveCompleted(run *SubagentRun) error {
	return s.SaveOutcome(run, SubagentOutcome{Ref: runRef(run), Status: SubagentOutcomeCompleted})
}

func (s *SubagentStore) SaveFailed(run *SubagentRun) error {
	return s.SaveOutcome(run, SubagentOutcome{Ref: runRef(run), Status: SubagentOutcomeFailed})
}

func (s *SubagentStore) SaveOutcome(run *SubagentRun, outcome SubagentOutcome) error {
	if s == nil || run == nil || run.Ref == "" || s.parentDestroyed(run) {
		return nil
	}
	branchErr := s.ensureBranchCreatedAt(run)
	var sessionErr error
	if run.Session != nil {
		sessionErr = run.Session.Save(s.sessionPath(run.Ref))
	}
	meta := run.Meta
	switch outcome.Status {
	case SubagentOutcomeCompleted:
		meta.Status = SubagentCompleted
	case SubagentOutcomeCancelled:
		meta.Status = SubagentInterrupted
	default:
		meta.Status = SubagentFailed
	}
	meta.Outcome = string(outcome.Status)
	meta.Retryable = outcome.Retryable
	meta.ErrorCode = outcome.ErrorCode
	meta.UpdatedAt = time.Now().UTC()
	run.Meta = meta
	return errors.Join(branchErr, sessionErr, s.saveMeta(meta))
}

func runRef(run *SubagentRun) string {
	if run == nil {
		return ""
	}
	return run.Ref
}
