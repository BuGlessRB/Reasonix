// Package workspacelease serializes Delivery writers that target the same
// workspace. Readers never acquire a lease. A writer keeps its lease from the
// first mutation until every participating agent run has finished, so review
// and verification cannot be invalidated by another Delivery session changing
// the workspace mid-turn. It never outlives those runs: a process a session
// leaves behind, a dev server or a watcher, is not a run, and an exclusion
// another session cannot outwait is worse than no exclusion at all.
package workspacelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const retryInterval = 75 * time.Millisecond

// waitNoticeGrace is how long a contended acquisition stays silent. Contention
// is either milliseconds, another session between two writes, or the length of
// a whole turn, and reporting the first kind leaves a permanent line about a
// wait nobody waited through. Lowered by tests.
var waitNoticeGrace = time.Second

var errHeld = errors.New("workspace write lease is held")

// WaitOutcome says which end of a contended acquisition a Wait reports.
type WaitOutcome int

const (
	// WaitBegan opens a wait that has already outlived waitNoticeGrace.
	WaitBegan WaitOutcome = iota
	// WaitAcquired closes one with the lease in hand.
	WaitAcquired
	// WaitAbandoned closes one without it: the caller's context ended first.
	WaitAbandoned
)

// Wait reports one contended acquisition. A wait under the grace is never
// reported at all, and a reported one always arrives as a pair, so nothing on
// screen is left claiming a wait that is already over.
type Wait struct {
	Outcome WaitOutcome
	Elapsed time.Duration
}

// WaitNotice receives both ends of a reported wait. It must return quickly and
// must not call back into Owner.
type WaitNotice func(Wait)

// Owner is one Delivery session's re-entrant workspace lease. One Owner may be
// shared by the root agent and all of its subagents. Different sessions must
// use different Owners, even when they share a workspace.
type Owner struct {
	lockPath string
	onWait   WaitNotice
	local    *localLock

	mu            sync.Mutex
	activeRuns    int
	acquired      bool
	acquiring     bool
	waiting       bool
	acquireDone   chan struct{}
	releaseSystem func()
}

// State is a sanitized process-local snapshot used by Desktop to explain a
// workspace conflict. It deliberately contains no path, PID, or lock token.
type State struct {
	Acquired bool
	Waiting  bool
}

// State returns the current acquisition state without performing lease I/O.
func (o *Owner) State() State {
	if o == nil {
		return State{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return State{Acquired: o.acquired, Waiting: o.waiting}
}

type localLock struct {
	token chan struct{}
}

var localRegistry = struct {
	sync.Mutex
	locks map[string]*localLock
}{locks: map[string]*localLock{}}

// New returns a Delivery-session lease owner for workspaceRoot. lockDir must be
// shared by Reasonix processes for cross-process protection; it is kept outside
// the workspace so acquiring a lease never dirties user files.
func New(workspaceRoot, lockDir string, onWait WaitNotice) (*Owner, error) {
	canonical, err := CanonicalWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	lockDir = strings.TrimSpace(lockDir)
	if lockDir == "" {
		return nil, errors.New("workspace lease directory is unavailable")
	}
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace lease directory: %w", err)
	}
	sum := sha256.Sum256([]byte(canonical))
	key := hex.EncodeToString(sum[:])

	localRegistry.Lock()
	local := localRegistry.locks[key]
	if local == nil {
		local = &localLock{token: make(chan struct{}, 1)}
		local.token <- struct{}{}
		localRegistry.locks[key] = local
	}
	localRegistry.Unlock()

	return &Owner{
		lockPath: filepath.Join(lockDir, key+".lock"),
		onWait:   onWait,
		local:    local,
	}, nil
}

// CanonicalWorkspace returns the stable identity used to key a workspace. It
// resolves symlinks when possible and folds case on Windows, where paths are
// case-insensitive by default.
func CanonicalWorkspace(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("workspace root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	abs = filepath.Clean(abs)
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = filepath.Clean(resolved)
	} else if !os.IsNotExist(resolveErr) {
		return "", fmt.Errorf("canonicalize workspace root: %w", resolveErr)
	}
	abs = nearestGitWorktreeRoot(abs)
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(filepath.ToSlash(abs))
	}
	return abs, nil
}

// nearestGitWorktreeRoot folds a repository root and any selected directory
// beneath it into one writer domain. It intentionally detects the .git marker
// through the filesystem instead of invoking Git, so the no-Git Windows path
// keeps the same safety guarantee. Linked worktrees each have their own .git
// marker and therefore remain independent writer domains.
func nearestGitWorktreeRoot(path string) string {
	start := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		start = filepath.Dir(path)
	}
	for current := start; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
	}
}

// BeginRun registers an agent run that participates in this session. The call
// is intentionally cheap and does not acquire the write lease; read-only turns
// therefore remain fully concurrent.
func (o *Owner) BeginRun() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.activeRuns++
	o.mu.Unlock()
}

// EndRun releases the lease once the final participating run finishes.
func (o *Owner) EndRun() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.activeRuns > 0 {
		o.activeRuns--
	}
	release := o.releaseIfIdleLocked()
	o.mu.Unlock()
	if release != nil {
		release()
	}
}

// AcquireWrite lazily acquires this session's exclusive write lease. It is
// re-entrant across parallel tool calls and shared subagents.
func (o *Owner) AcquireWrite(ctx context.Context) error {
	if o == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		o.mu.Lock()
		if o.acquired {
			o.mu.Unlock()
			return nil
		}
		if o.acquiring {
			done := o.acquireDone
			o.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		o.acquiring = true
		o.acquireDone = make(chan struct{})
		done := o.acquireDone
		o.mu.Unlock()

		release, err := o.acquire(ctx)
		o.mu.Lock()
		o.acquiring = false
		o.waiting = false
		if err == nil {
			o.acquired = true
			o.releaseSystem = release
		}
		close(done)
		releaseIfIdle := o.releaseIfIdleLocked()
		o.mu.Unlock()
		if releaseIfIdle != nil {
			releaseIfIdle()
		}
		return err
	}
}

func (o *Owner) releaseIfIdleLocked() func() {
	if !o.acquired || o.acquiring || o.activeRuns != 0 {
		return nil
	}
	release := o.releaseSystem
	o.acquired = false
	o.releaseSystem = nil
	return release
}

func (o *Owner) notify(w Wait) {
	if o.onWait != nil {
		o.onWait(w)
	}
}

func (o *Owner) markWaiting() {
	o.mu.Lock()
	o.waiting = true
	o.mu.Unlock()
}

// waitClock reports both ends of one contended acquisition, or neither: a wait
// that clears inside the grace never becomes a line someone has to read, and
// one that does not is always closed by the report that ends it.
type waitClock struct {
	owner   *Owner
	started time.Time
	began   bool
}

func (w *waitClock) contend() {
	if w.started.IsZero() {
		w.started = time.Now()
		w.owner.markWaiting()
	}
}

func (w *waitClock) report() {
	if w.began || w.started.IsZero() || time.Since(w.started) < waitNoticeGrace {
		return
	}
	w.began = true
	w.owner.notify(Wait{Outcome: WaitBegan, Elapsed: time.Since(w.started)})
}

func (w *waitClock) close(outcome WaitOutcome) {
	if w.began {
		w.owner.notify(Wait{Outcome: outcome, Elapsed: time.Since(w.started)})
	}
}

func (w *waitClock) remainingGrace() time.Duration {
	if left := waitNoticeGrace - time.Since(w.started); left > 0 {
		return left
	}
	return time.Nanosecond
}

// awaitToken waits for the in-process token. The grace timer is dropped once
// the report is out, so a long wait stops waking to re-decide it.
func (o *Owner) awaitToken(ctx context.Context, w *waitClock) error {
	w.contend()
	timer := time.NewTimer(w.remainingGrace())
	defer timer.Stop()
	grace := timer.C
	for {
		select {
		case <-o.local.token:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-grace:
			w.report()
			grace = nil
		}
	}
}

func (o *Owner) acquire(ctx context.Context) (func(), error) {
	w := &waitClock{owner: o}
	select {
	case <-o.local.token:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		if err := o.awaitToken(ctx, w); err != nil {
			w.close(WaitAbandoned)
			return nil, err
		}
	}

	releaseLocal := func() { o.local.token <- struct{}{} }
	for {
		releaseFile, err := tryLockFile(o.lockPath)
		if err == nil {
			w.close(WaitAcquired)
			return func() {
				releaseFile()
				releaseLocal()
			}, nil
		}
		if !errors.Is(err, errHeld) {
			releaseLocal()
			w.close(WaitAbandoned)
			return nil, fmt.Errorf("acquire workspace write lease: %w", err)
		}
		w.contend()
		w.report()
		timer := time.NewTimer(retryInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			releaseLocal()
			w.close(WaitAbandoned)
			return nil, ctx.Err()
		}
	}
}
