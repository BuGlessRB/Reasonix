package winsandbox

import (
	"errors"
	"os"
	"time"
)

// ErrUnsupported is returned when the package is used on a non-Windows host or
// when required native Windows sandbox APIs are unavailable.
var ErrUnsupported = errors.New("windows sandbox is unavailable")

// FailurePhase identifies failures owned by the sandbox runner before the
// requested process begins executing. Errors after ResumeThread intentionally
// remain untyped because the command may already have mutated state.
type FailurePhase string

const (
	FailureAuthorization FailurePhase = "authorization"
	FailureDependency    FailurePhase = "dependency"
	FailureLaunch        FailurePhase = "launch"
)

type phasedFailure struct {
	phase FailurePhase
	err   error
}

func (e phasedFailure) Error() string { return e.err.Error() }
func (e phasedFailure) Unwrap() error { return e.err }

func failureAt(phase FailurePhase, err error) error {
	if err == nil {
		return nil
	}
	return phasedFailure{phase: phase, err: err}
}

// FailurePhaseOf reports the typed pre-command stage attached by the native
// runner. Callers must not infer a stage from an exit code alone.
func FailurePhaseOf(err error) (FailurePhase, bool) {
	var failure phasedFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	switch failure.phase {
	case FailureAuthorization, FailureDependency, FailureLaunch:
		return failure.phase, true
	default:
		return "", false
	}
}

var _ error = phasedFailure{}

// Spec describes one native Windows sandbox launch.
//
// Direct read-only launches use AppContainer. Shell and writer launches use a
// WRITE_RESTRICTED token whose restricting SIDs carry only the selected
// directory capabilities. ForbidReadRoots are denied with temporary deny ACEs.
// Network=false is supported for AppContainer launches; restricted-token
// launches fail closed because WRITE_RESTRICTED does not isolate network.
type Spec struct {
	ReadableRoots       []string
	WritableRoots       []string
	ForbidReadRoots     []string
	ProtectedWriteRoots []string
	Network             bool
	Writable            bool
	ReadOnly            bool
	TempDir             string
	TempPrefix          string
	// LockWait bounds how long this run may queue behind another sandboxed
	// command holding the same per-root lock before failing with a clear
	// error. Zero uses the short interactive default; callers whose run
	// nobody is blocked on (background jobs) pass a longer budget.
	// WINDOWS_SANDBOX_LOCK_MS overrides both.
	LockWait time.Duration
}

// RunOptions carries process IO and environment overrides.
type RunOptions struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
	Env    []string
	Dir    string
}

// Result is the completed sandboxed process result.
type Result struct {
	ExitCode int
}
