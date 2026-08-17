package agent

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"reasonix/internal/checkpoint"
	"reasonix/internal/evidence"
	"reasonix/internal/tool"
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
	state map[string]watchedPath // keyed by evidence.NormalizePath
}

// watchedPath keeps the spelling the call used beside the state, because the map
// is keyed by the ledger's path identity: the same file reaches here from the
// ledger and from a directory read, and on Windows those differ in case only.
type watchedPath struct {
	path string
	pathState
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
	snap := pathSnapshot{root: root, state: make(map[string]watchedPath, len(paths))}
	for _, p := range paths {
		key := evidence.NormalizePath(p)
		if key == "" {
			continue
		}
		if _, seen := snap.state[key]; seen {
			continue
		}
		snap.state[key] = watchedPath{path: p, pathState: statePathOf(p)}
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
	for _, was := range before.state {
		now := statePathOf(was.path)
		if now == was.pathState {
			continue
		}
		affected = append(affected, was.path)
		if !was.exists && now.exists {
			created = append(created, was.path)
		}
	}
	for _, path := range workspaceTopLevel(before.root) {
		if _, watched := before.state[evidence.NormalizePath(path)]; watched {
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
		if !holdsPath(rec.Paths, path) {
			rec.Paths = append(rec.Paths, path)
		}
	}
}

// holdsPath asks whether the receipt already names this file, by the ledger's
// identity rather than by spelling — otherwise a receipt lists one Windows file
// twice and every count downstream reads one change as two.
func holdsPath(paths []string, want string) bool {
	key := evidence.NormalizePath(want)
	for _, p := range paths {
		if evidence.NormalizePath(p) == key {
			return true
		}
	}
	return false
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

// touchedTheWorkspace reports whether a change landed in the work product. A
// probe written under $TMPDIR is outside it by construction, so a turn whose
// only write was a scratch file owes no verification of the workspace — the
// same reasoning the memory-write case already carries, read off the path.
func (a *Agent) touchedTheWorkspace(r evidence.Receipt) bool {
	if a.writeWorkspaceRoot == "" || len(r.Paths) == 0 {
		return true
	}
	return slices.ContainsFunc(r.Paths, a.pathInWorkspace)
}

// pathInWorkspace reports whether one path is part of the work product. A
// relative path is resolved against the workspace by every file tool, so it is
// inside by construction; only an absolute one can leave.
func (a *Agent) pathInWorkspace(path string) bool {
	if a.writeWorkspaceRoot == "" {
		return true
	}
	return !filepath.IsAbs(path) || pathWithinFold(a.writeWorkspaceRoot, path)
}

// mutationBaseline is what the turn's remaining obligations are measured from:
// the latest change still on disk. A turn whose only writes were scratch files
// and build artifacts it cleaned up has no baseline, and owes no verification
// of changes it kept none of.
func (a *Agent) mutationBaseline(delivery bool) (int, bool) {
	ledger := a.task.ledger
	survives := func(r evidence.Receipt) bool {
		return a.touchedTheWorkspace(r) && leftSomethingBehind(ledger, r)
	}
	if delivery {
		return ledger.LatestProvenMutationIndexFunc(survives)
	}
	return ledger.LatestSuccessfulWriterIndexFunc(survives)
}

// observeBeforeMutation captures preimages for Previewable writers and records
// explicit coverage gaps for bash / opaque MCP tools. Host-internal only.
func (a *Agent) observeBeforeMutation(ctx context.Context, plan *toolCallPlan) {
	if a == nil || plan == nil {
		return
	}
	toolName := plan.evidenceName
	if toolName == "" {
		toolName = plan.call.Name
	}
	obs := a.svc.mutationObserver
	if obs != nil {
		if pv, ok := plan.execTool.(tool.Previewer); ok {
			if change, perr := pv.Preview(ctx, plan.execArgs); perr == nil && change.Path != "" {
				obs.BeforeMutationFromChange(change, toolName)
				plan.mutationPath = change.Path
				plan.mutationWitness = evidence.ChangedLines(change.OldText, change.NewText)
				return
			}
		}
		// Non-previewable writers: record a coverage gap (do not guess paths).
		switch toolName {
		case "bash":
			obs.RecordGap(checkpoint.CoverageGap{Reason: checkpoint.GapBashSideEffect, Tool: toolName, Detail: "bash side effects are not path-tracked"})
		default:
			// MCP or other writers without Previewer.
			if !plan.readOnly {
				obs.RecordGap(checkpoint.CoverageGap{Reason: checkpoint.GapMCPExternal, Tool: toolName, Detail: "tool cannot describe local write paths"})
			}
		}
		return
	}
	// Legacy onPreEdit path.
	if a.svc.preEdit != nil {
		if pv, ok := plan.execTool.(tool.Previewer); ok {
			if change, perr := pv.Preview(ctx, plan.execArgs); perr == nil {
				a.svc.preEdit(change)
				plan.mutationPath = change.Path
			}
		}
	}
}

// observeAfterMutation records the after fingerprint when a concrete path was
// known before execution, regardless of tool success or failure.
func (a *Agent) observeAfterMutation(plan *toolCallPlan) {
	if a == nil || plan == nil || plan.mutationPath == "" || a.svc.mutationObserver == nil {
		return
	}
	toolName := plan.evidenceName
	if toolName == "" {
		toolName = plan.call.Name
	}
	a.svc.mutationObserver.AfterMutation(plan.mutationPath, toolName)
}

// workspaceScanLimit bounds the walk. Past it the scan reports itself
// incomplete and nothing is concluded from it, so a very large workspace keeps
// the behaviour it has today instead of paying for an answer it cannot trust.
const workspaceScanLimit = 50_000

// vcsStoreDirs are the repositories' own stores. They are not the work product
// — `git status` alone rewrites the index — and a change there says nothing
// about whether a command touched the tree.
var vcsStoreDirs = map[string]bool{".git": true, ".hg": true, ".svn": true}

// workspaceScan is every file under the workspace at one moment. complete is
// false when the walk was cut short, which is the only honest answer a partial
// scan can give: "changed nothing" cannot be read off a tree half-looked-at.
type workspaceScan struct {
	state    map[string]pathState
	complete bool
}

// scanWorkspace walks the whole tree rather than an ignore-filtered part of it:
// npm install and a build write exactly where ignore rules point away, so a
// filtered walk would report the biggest writes there are as no change at all.
func scanWorkspace(root string) workspaceScan {
	if root == "" {
		return workspaceScan{}
	}
	state := make(map[string]pathState, 4096)
	complete := true
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			complete = false
			return nil
		}
		if d.IsDir() {
			if vcsStoreDirs[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if len(state) >= workspaceScanLimit {
			complete = false
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			complete = false
			return nil
		}
		state[path] = pathState{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		complete = false
	}
	return workspaceScan{state: state, complete: complete}
}

// unchanged reports whether the workspace is byte-for-byte as this scan found
// it. Both scans must be complete; either one short of that proves nothing.
func (before workspaceScan) unchanged(after workspaceScan) bool {
	if !before.complete || !after.complete || len(before.state) != len(after.state) {
		return false
	}
	for path, was := range before.state {
		if now, ok := after.state[path]; !ok || now != was {
			return false
		}
	}
	return true
}

// settleUnchangedWorkspace answers by observation what no reading of a command
// can: whether it changed anything. A call the host could not classify — sed
// through its own script language, a wrapper, a path built from a variable —
// stays a mutation unless the workspace it ran against is exactly as it was.
func (a *Agent) settleUnchangedWorkspace(rec *evidence.Receipt, plan *toolCallPlan) {
	if rec == nil || plan == nil || !rec.Success || rec.MutationEvidence != evidence.MutationUnknown {
		return
	}
	if !plan.scanBefore.complete || !plan.scanBefore.unchanged(scanWorkspace(a.writeWorkspaceRoot)) {
		return
	}
	rec.Mutation = false
	rec.MutationEvidence = ""
}

// scanBeforeUnprovenCall takes the whole-workspace scan only for a call the
// host could not classify. A proven writer already reports its paths, and a
// proven reader has nothing to settle, so neither pays for the walk.
func (a *Agent) scanBeforeUnprovenCall(plan *toolCallPlan) workspaceScan {
	if evidence.ToolCallMutationClass(plan.evidenceName, plan.evidenceArgs, plan.readOnly) != evidence.MutationUnknown {
		return workspaceScan{}
	}
	// A backgrounded job's receipt is written when it starts, so the workspace
	// it will write to still looks untouched. It never settles.
	if isBackgroundTaskCall(string(plan.evidenceArgs)) {
		return workspaceScan{}
	}
	return scanWorkspace(a.writeWorkspaceRoot)
}
