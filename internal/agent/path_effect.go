package agent

import (
	"os"
	"slices"

	"reasonix/internal/evidence"
)

// A command the host cannot decompose statically — `rm x && ls`, a project
// script — is unknown, which is honest but final: the ledger then carries a
// mutation with no paths. Watching the call closes that gap without teaching
// the host any command's semantics. Only already-touched paths are watched, so
// a call that changed something else stays unknown, never a wrong "nothing".
const observedPathLimit = 32

type pathState struct {
	exists  bool
	size    int64
	modTime int64
}

type pathSnapshot map[string]pathState

// snapshotPaths records the current state of the call's own targets plus the
// paths this turn has already read or written. Take it as the last step before
// the call runs, so hooks and approvals that touch the workspace stay outside
// what the call is credited with.
func snapshotPaths(ledger *evidence.Ledger, targets []string) pathSnapshot {
	paths := append([]string(nil), targets...)
	paths = append(paths, ledger.TouchedPaths(observedPathLimit, false)...)
	if len(paths) == 0 {
		return nil
	}
	snap := make(pathSnapshot, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		snap[p] = statePathOf(p)
	}
	return snap
}

func statePathOf(path string) pathState {
	info, err := os.Lstat(path)
	if err != nil {
		return pathState{}
	}
	return pathState{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
}

// since compares the snapshot against the paths as they are now, returning what
// the call changed and, of those, what it brought into existence.
func (before pathSnapshot) since() (affected, created []string) {
	for path, was := range before {
		now := statePathOf(path)
		if now == was {
			continue
		}
		affected = append(affected, path)
		if !was.exists && now.exists {
			created = append(created, path)
		}
	}
	slices.Sort(affected)
	slices.Sort(created)
	return affected, created
}

// decorateObservedPaths records what a call demonstrably did. Classification is
// only ever sharpened: a call observed to change nothing stays unknown, since
// it may have changed something outside the watched set.
func decorateObservedPaths(rec *evidence.Receipt, plan *toolCallPlan) {
	if rec == nil || plan == nil || len(plan.pathsBefore) == 0 || !rec.Success {
		return
	}
	affected, created := plan.pathsBefore.since()
	rec.Created = created
	if rec.MutationEvidence != evidence.MutationUnknown || len(affected) == 0 {
		return
	}
	rec.MutationEvidence = evidence.MutationProven
	for _, path := range affected {
		if !slices.Contains(rec.Paths, path) {
			rec.Paths = append(rec.Paths, path)
		}
	}
}

// leftSomethingBehind reports whether a write still has anything on disk to
// verify. Only a path the host watched appear and then watched vanish counts as
// cleaned up — anything it did not see created is assumed to have survived,
// since a path it cannot resolve looks exactly like one that is gone.
func leftSomethingBehind(r evidence.Receipt) bool {
	if len(r.Created) == 0 {
		return true
	}
	for _, path := range r.Paths {
		if !slices.Contains(r.Created, path) {
			return true
		}
	}
	for _, path := range r.Created {
		if statePathOf(path).exists {
			return true
		}
	}
	return false
}

// mutationBaseline is what the turn's remaining obligations are measured from:
// the latest change still on disk. A turn whose only writes were scratch files
// it cleaned up has no baseline, and owes no verification of changes it kept
// none of.
func (a *Agent) mutationBaseline(delivery bool) (int, bool) {
	if delivery {
		return a.task.ledger.LatestProvenMutationIndexFunc(leftSomethingBehind)
	}
	return a.task.ledger.LatestSuccessfulWriterIndexFunc(leftSomethingBehind)
}
