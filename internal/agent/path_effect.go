package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"reasonix/internal/evidence"
)

// A command the host cannot decompose statically — `rm x && ls`, a project
// script — is unknown, which is honest but final: the ledger then carries a
// mutation with no paths. Watching the call closes that gap without teaching
// the host any command's semantics. Only watched paths can be reported, so a
// call that changed something else stays unknown, never a wrong "nothing".
const observedPathLimit = 32

type pathState struct {
	exists  bool
	size    int64
	modTime int64
}

// pathSnapshot is what the workspace looked like before a call. root is kept so
// the comparison can also find entries that did not exist to be watched — a
// build artifact appears under a name no tool ever passed to the host.
type pathSnapshot struct {
	root  string
	state map[string]pathState
}

func (before pathSnapshot) empty() bool { return len(before.state) == 0 && before.root == "" }

// snapshotPaths records the call's own targets, the paths this turn has already
// read or written, and the workspace's top level. Take it as the last step
// before the call runs, so hooks and approvals that touch the workspace stay
// outside what the call is credited with.
func snapshotPaths(ledger *evidence.Ledger, root string, targets []string) pathSnapshot {
	root = strings.TrimSpace(root)
	paths := append([]string(nil), targets...)
	paths = append(paths, ledger.TouchedPaths(observedPathLimit, false)...)
	paths = append(paths, workspaceTopLevel(root)...)
	if len(paths) == 0 && root == "" {
		return pathSnapshot{}
	}
	snap := pathSnapshot{root: root, state: make(map[string]pathState, len(paths))}
	for _, p := range paths {
		if p != "" {
			snap.state[p] = statePathOf(p)
		}
	}
	return snap
}

// workspaceTopLevel lists the workspace's immediate entries — one directory
// read, never recursive, so a repository with a large tree costs the same as an
// empty one.
func workspaceTopLevel(root string) []string {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > observedPathLimit*4 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.Join(root, e.Name()))
	}
	return out
}

func statePathOf(path string) pathState {
	info, err := os.Lstat(path)
	if err != nil {
		return pathState{}
	}
	return pathState{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
}

// since compares the snapshot against the workspace as it is now, returning
// what the call changed and, of those, what it brought into existence.
func (before pathSnapshot) since() (affected, created []string) {
	for path, was := range before.state {
		now := statePathOf(path)
		if now == was {
			continue
		}
		affected = append(affected, path)
		if !was.exists && now.exists {
			created = append(created, path)
		}
	}
	for _, path := range workspaceTopLevel(before.root) {
		if _, watched := before.state[path]; watched {
			continue
		}
		affected = append(affected, path)
		created = append(created, path)
	}
	slices.Sort(affected)
	slices.Sort(created)
	return affected, created
}

// decorateObservedPaths records what a call demonstrably did. Classification is
// only ever sharpened: a call observed to change nothing stays unknown, since
// it may have changed something outside the watched set.
func decorateObservedPaths(rec *evidence.Receipt, plan *toolCallPlan) {
	if rec == nil || plan == nil || plan.pathsBefore.empty() || !rec.Success {
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

// leftSomethingBehind reports whether a change still has anything on disk to
// verify. A path that is gone counts as cleaned up only if this turn created
// it: removing a file the turn made leaves the workspace as found, removing one
// it did not is the change. An unresolvable path looks exactly like a deleted
// one, so anything never watched appear is assumed to have survived.
func leftSomethingBehind(ledger *evidence.Ledger, r evidence.Receipt) bool {
	if len(r.Paths) == 0 {
		return true
	}
	for _, path := range r.Paths {
		if statePathOf(path).exists || !ledger.CreatedInTurn(path) {
			return true
		}
	}
	return false
}

// mutationBaseline is what the turn's remaining obligations are measured from:
// the latest change still on disk. A turn whose only writes were scratch files
// and build artifacts it cleaned up has no baseline, and owes no verification
// of changes it kept none of.
func (a *Agent) mutationBaseline(delivery bool) (int, bool) {
	ledger := a.task.ledger
	survives := func(r evidence.Receipt) bool { return leftSomethingBehind(ledger, r) }
	if delivery {
		return ledger.LatestProvenMutationIndexFunc(survives)
	}
	return ledger.LatestSuccessfulWriterIndexFunc(survives)
}
