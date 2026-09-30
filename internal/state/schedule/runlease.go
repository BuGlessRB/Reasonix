package schedule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/base/filelock"
)

const leasesDirName = "leases"

func (s *Store) leasePath(triggerID string) string {
	return filepath.Join(s.dir, leasesDirName, strings.TrimSuffix(resultName(triggerID), ".json")+".lock")
}

// HoldRun takes the exclusive operating-system lock that says a run is alive.
// Of any number of executors started for one claim exactly one gets it; the
// rest get ErrRunHeld. The lock dies with its holder, so a kill or a crash frees
// it without anyone having to clean up, which is why the reaper reads it and no
// file content.
func (s *Store) HoldRun(triggerID string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Join(s.dir, leasesDirName), 0o700); err != nil {
		return nil, fmt.Errorf("schedule: leases dir: %w", err)
	}
	release, err = filelock.TryAcquire(s.leasePath(triggerID))
	if errors.Is(err, filelock.ErrHeld) {
		return nil, ErrRunHeld
	}
	if err != nil {
		return nil, fmt.Errorf("schedule: run lease: %w", err)
	}
	return release, nil
}

// MarkStarted records, once and durably, that an executor began the run's work.
// A second call for the same run gets ErrRunStarted, whoever makes it and
// however long after: a run that died half-way is settled by the reaper and never
// executed again, and nothing that can start the child by hand can spend the
// same claim twice. It is called under HoldRun.
func (s *Store) MarkStarted(triggerID string) error {
	if err := os.MkdirAll(filepath.Join(s.dir, leasesDirName), 0o700); err != nil {
		return fmt.Errorf("schedule: leases dir: %w", err)
	}
	f, err := os.OpenFile(s.startedPath(triggerID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return ErrRunStarted
	}
	if err != nil {
		return fmt.Errorf("schedule: start marker: %w", err)
	}
	return f.Close()
}

func (s *Store) startedPath(triggerID string) string {
	return filepath.Join(s.dir, leasesDirName, strings.TrimSuffix(resultName(triggerID), ".json")+".started")
}

// RunHeld reports whether some executor holds the run's lease right now. A lease
// that cannot be probed counts as held: a run is never reaped on a guess.
func (s *Store) RunHeld(r Run) bool {
	if err := os.MkdirAll(filepath.Join(s.dir, leasesDirName), 0o700); err != nil {
		return true
	}
	release, err := filelock.TryAcquire(s.leasePath(r.TriggerID))
	if err != nil {
		return true
	}
	release()
	return false
}

// ReapDead settles every in-flight run whose executor is gone, deciding from the
// operating-system lease alone, and drops lease files no record refers to.
func (s *Store) ReapDead(ctx context.Context, p Policy) ([]string, error) {
	reaped, err := s.Reap(ctx, p, s.RunHeld)
	if m, _, snapErr := s.Snapshot(ctx); snapErr == nil {
		s.pruneLeases(m)
	}
	return reaped, err
}

func (s *Store) pruneLeases(m Manifest) {
	entries, err := os.ReadDir(filepath.Join(s.dir, leasesDirName))
	if err != nil {
		return
	}
	live := map[string]bool{}
	for _, r := range m.Runs {
		live[filepath.Base(s.leasePath(r.TriggerID))] = true
	}
	cutoff := s.clock().Add(-ReapGrace)
	for _, e := range entries {
		if live[strings.TrimSuffix(e.Name(), ".started")+".lock"] {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.dir, leasesDirName, e.Name()))
		}
	}
}
