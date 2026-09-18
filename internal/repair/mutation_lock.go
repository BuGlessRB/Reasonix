package repair

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/filelock"
	"reasonix/internal/pathidentity"
)

const repairMutationLockTimeout = 5 * time.Second

// repairMutationBeforeLock is a test seam for forcing competing repair
// operations to overlap before one waits on the shared file lock.
var repairMutationBeforeLock = func([]string) {}

// repairMutationBeforeRename is a test seam for changing a target in the
// narrow interval between its final state check and quarantine rename.
var repairMutationBeforeRename = func(string) {}

// repairMutationAfterRename is a test seam for forcing an uncooperative writer
// to create a new target after the confirmed node has been quarantined.
var repairMutationAfterRename = func(string) {}

// repairMutationAfterPrepare is a test seam for simulating process exit after
// the write-ahead repair intent is durable but before the filesystem rename.
var repairMutationAfterPrepare = func(string) {}

var repairPathCaseInsensitive = platformRepairPathCaseInsensitive

func lockRepairTransaction() (func(), error) {
	expectedPendingState := repairPlanReleaseNodeState(pendingRepairTransactionPath())
	unlock, err := lockRepairMutationProtocolFile(repairTransactionPath())
	if err != nil {
		return nil, fmt.Errorf("lock repair transaction: %w", err)
	}
	if actual := repairPlanReleaseNodeState(pendingRepairTransactionPath()); actual != expectedPendingState {
		unlock()
		return nil, fmt.Errorf("lock repair transaction: pending repair transaction changed while waiting")
	}
	return unlock, nil
}

func restoreRepairNodeIfAbsent(backup, target string) error {
	// Every backup passed here was produced by renaming the target to a sibling
	// or to a same-filesystem repair directory. A no-replace rename restores the
	// exact node and consumes the backup in one operation. Recreating a link/file
	// and then removing backup would let another writer replace backup between
	// those syscalls and have its node deleted.
	if err := renameRepairNodeNoReplace(backup, target); err != nil {
		return fmt.Errorf("restore repair target: %w", err)
	}
	return nil
}

// removeRepairNodeIfMatching first displaces a transaction-owned backup to a
// unique sibling, then verifies the moved node against its original target
// identity. A path replacement between verification and cleanup is restored or
// retained, never unlinked as if it were transaction-owned.
func removeRepairNodeIfMatching(path, identityPath, expectedStateID string) error {
	expectedStateID = strings.TrimSpace(expectedStateID)
	if expectedStateID == "" {
		// Legacy transactions did not persist backup identity. Leaving a stale
		// backup is safer than deleting a path whose ownership cannot be proven.
		return nil
	}
	cleanup, err := moveRepairNodeToUniqueCleanup(path)
	if err != nil {
		return err
	}
	if cleanup == "" {
		return nil
	}
	if err := verifyRepairPlanReleaseNodeStateFor(cleanup, identityPath, expectedStateID); err != nil {
		if restoreErr := renameRepairNodeNoReplace(cleanup, path); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("preserve changed repair backup at %s: %w", cleanup, restoreErr))
		}
		return err
	}
	info, err := os.Lstat(cleanup)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if restoreErr := renameRepairNodeNoReplace(cleanup, path); restoreErr != nil {
			return errors.Join(
				fmt.Errorf("remove repair backup: directories are unsupported"),
				fmt.Errorf("preserve repair backup at %s: %w", cleanup, restoreErr),
			)
		}
		return fmt.Errorf("remove repair backup: directories are unsupported")
	}
	return os.Remove(cleanup)
}

func moveRepairNodeToUniqueCleanup(path string) (string, error) {
	for attempt := range 16 {
		cleanup := fmt.Sprintf("%s.reasonix-cleanup-%d-%d", path, time.Now().UTC().UnixNano(), attempt)
		err := renameRepairNodeNoReplace(path, cleanup)
		if err == nil {
			return cleanup, nil
		}
		if os.IsNotExist(err) {
			return "", nil
		}
		if os.IsExist(err) {
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("remove repair node: cannot allocate cleanup path")
}

// canonicalRepairPath resolves a repair target to a stable key shared by
// mutation locks and preview identity. Parent-directory symlinks are followed
// so alias paths converge, but the leaf name is never resolved: repair mutates
// the leaf node itself via Lstat/Rename (including when the leaf is a symlink).
// Case-insensitive filesystems fold case so /Project and /project cannot take
// different locks. The decision is made from the target's actual parent
// directory: macOS and Windows can both host case-sensitive directories.
func canonicalRepairPath(path string) string {
	identity, err := pathidentity.Resolve(path, pathidentity.Options{FollowLeaf: false})
	if err != nil {
		return ""
	}
	return identity.Key
}

// legacyCanonicalRepairPath freezes the lock and persisted-target identity used
// before path identity v2. New writers acquire both names during the supported
// cross-version window.
func legacyCanonicalRepairPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		absolute = filepath.Clean(path)
	}
	absolute = resolveParentSymlinkPath(absolute)
	absolute = filepath.Clean(absolute)
	caseInsensitive := repairPathCaseInsensitive(absolute)
	absolute = platformRepairPathUnicodeNormalized(absolute)
	if caseInsensitive {
		return strings.ToLower(filepath.ToSlash(absolute))
	}
	return absolute
}

// resolveParentSymlinkPath resolves symlink parents of path and re-attaches the
// original leaf base name. The leaf is intentionally not EvalSymlinks'd: two
// different symlink leaves that share a referent must stay distinct targets.
func resolveParentSymlinkPath(path string) string {
	if path == "" {
		return ""
	}
	parent := filepath.Dir(path)
	base := filepath.Base(path)
	if parent == path {
		// Root or volume path: nothing to resolve above the leaf.
		return path
	}
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(resolved, base)
	}
	// Parent may not exist yet (create-only targets). Resolve the longest
	// existing ancestor and rejoin the missing components including the leaf.
	var missing []string
	dir := parent
	missing = append(missing, base)
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			parts := make([]string, 0, 1+len(missing))
			parts = append(parts, resolved)
			for _, v := range slices.Backward(missing) {
				parts = append(parts, v)
			}
			return filepath.Join(parts...)
		}
		next := filepath.Dir(dir)
		if next == dir {
			return path
		}
		missing = append(missing, filepath.Base(dir))
		dir = next
	}
}

// LockRepairMutations is the exported form of lockRepairMutations for desktop
// handoff helpers that replace release-unit paths outside ApplyRepairPlan.
func LockRepairMutations(paths ...string) (func(), error) {
	return lockRepairMutations(paths...)
}

// LockRepairMutationsTimeout is like LockRepairMutations but waits up to
// timeout for competing repair or update holders.
func LockRepairMutationsTimeout(timeout time.Duration, paths ...string) (func(), error) {
	if timeout <= 0 {
		timeout = repairMutationLockTimeout
	}
	return lockRepairMutationsTimeout(timeout, paths...)
}

// repairPlanTargetIdentity is a non-reversible identity for a filesystem
// target. It is embedded in preview state IDs so confirmation cannot be
// reused against a different real path that happens to have the same content.
func repairPlanTargetIdentity(path string) string {
	key := canonicalRepairPath(path)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// lockRepairMutations serializes repair read-check-write cycles by canonical
// target path. Lock files live in Reasonix state rather than beside project or
// configuration files, and paths are sorted so multi-target actions cannot
// deadlock each other.
func lockRepairMutations(paths ...string) (func(), error) {
	return lockRepairMutationsTimeoutMode(repairMutationLockTimeout, true, paths...)
}

func lockRepairMutationsTimeout(timeout time.Duration, paths ...string) (func(), error) {
	return lockRepairMutationsTimeoutMode(timeout, true, paths...)
}

// Protocol files are expected to be atomically replaced by the previous lock
// holder. Their callers compare content state after acquisition, so only the
// lock domain is shared here; ordinary repair targets still require native
// file identity to remain unchanged while waiting.
func lockRepairMutationProtocolFile(path string) (func(), error) {
	return lockRepairMutationsTimeoutMode(repairMutationLockTimeout, false, path)
}

func lockRepairMutationsTimeoutMode(timeout time.Duration, revalidateTargets bool, paths ...string) (func(), error) {
	lockDir := config.RepairMutationLockDir()
	if lockDir == "" {
		return nil, fmt.Errorf("lock repair mutations: OS user cache directory is unavailable")
	}
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, fmt.Errorf("lock repair mutations: create lock directory: %w", err)
	}

	type targetIdentity struct {
		path   string
		key    string
		info   os.FileInfo
		exists bool
	}
	targets := make([]targetIdentity, 0, len(paths))
	primaryUnique := map[string]struct{}{}
	lockKeySet := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		identity, err := pathidentity.Resolve(path, pathidentity.Options{FollowLeaf: false})
		if err != nil {
			return nil, fmt.Errorf("lock repair mutations: resolve target: %w", err)
		}
		if _, ok := primaryUnique[identity.Key]; ok {
			continue
		}
		info, statErr := os.Lstat(identity.AccessPath)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("lock repair mutations: inspect target: %w", statErr)
		}
		primaryUnique[identity.Key] = struct{}{}
		targets = append(targets, targetIdentity{path: identity.AccessPath, key: identity.Key, info: info, exists: exists})
		lockKeySet[identity.Key] = struct{}{}
		if legacy := legacyCanonicalRepairPath(path); legacy != "" {
			lockKeySet[legacy] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return func() {}, nil
	}
	primaryKeys := make([]string, 0, len(primaryUnique))
	for key := range primaryUnique {
		primaryKeys = append(primaryKeys, key)
	}
	sort.Strings(primaryKeys)
	repairMutationBeforeLock(append([]string(nil), primaryKeys...))
	lockPaths := make([]string, 0, len(lockKeySet))
	for key := range lockKeySet {
		digest := sha256.Sum256([]byte(key))
		lockPaths = append(lockPaths, filepath.Join(lockDir, fmt.Sprintf("%x.lock", digest)))
	}
	sort.Strings(lockPaths)

	if timeout <= 0 {
		timeout = repairMutationLockTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var releases []func()
	for {
		releases = releases[:0]
		contended := false
		for _, lockPath := range lockPaths {
			release, acquireErr := filelock.TryAcquire(lockPath)
			if acquireErr == nil {
				releases = append(releases, release)
				continue
			}
			for _, held := range slices.Backward(releases) {
				held()
			}
			releases = releases[:0]
			if !errors.Is(acquireErr, filelock.ErrHeld) {
				return nil, fmt.Errorf("lock repair mutations: %w", acquireErr)
			}
			contended = true
			break
		}
		if !contended {
			break
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, fmt.Errorf("lock repair mutations: %w", ctx.Err())
		}
	}
	if revalidateTargets {
		for _, target := range targets {
			identity, resolveErr := pathidentity.Resolve(target.path, pathidentity.Options{FollowLeaf: false})
			currentInfo, statErr := os.Lstat(target.path)
			currentExists := statErr == nil
			if statErr != nil && !os.IsNotExist(statErr) && resolveErr == nil {
				resolveErr = statErr
			}
			identityChanged := resolveErr == nil && (identity.Key != target.key || repairEntryRedirected(target.info, target.exists, currentInfo, currentExists))
			if resolveErr != nil || identityChanged {
				for _, held := range slices.Backward(releases) {
					held()
				}
				if resolveErr != nil {
					return nil, fmt.Errorf("lock repair mutations: revalidate target: %w", resolveErr)
				}
				return nil, fmt.Errorf("lock repair mutations: target identity changed while waiting")
			}
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			for _, v := range slices.Backward(releases) {
				v()
			}
		})
	}, nil
}

func repairEntryRedirected(before os.FileInfo, beforeExists bool, after os.FileInfo, afterExists bool) bool {
	// Regular files are commonly published with rename(2) while holding this
	// lock. A waiter still owns the same parent directory and leaf name after
	// that replacement; the caller's content/hash precondition detects stale
	// state. Directory entries, links, and special nodes retain native identity
	// because replacing one can redirect the authorized operation elsewhere.
	if (!beforeExists || before.Mode().IsRegular()) && (!afterExists || after.Mode().IsRegular()) {
		return false
	}
	return beforeExists != afterExists || !os.SameFile(before, after)
}
