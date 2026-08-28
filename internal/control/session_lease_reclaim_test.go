package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/testenv"
)

func leasePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testenv.TempDir(t), name+".jsonl")
}

// The refusal a stranded lease produces names this very process, so telling
// the user to close the other window sends them looking for a window that does
// not exist. Recovering our own leftover is the only way out that does not ask
// them to delete files.
func TestReclaimOwnSessionLeaseTakesOverThisProcessLeftover(t *testing.T) {
	path := leasePath(t, "own")
	held := &agent.SessionLeaseError{
		Path: path,
		Info: &agent.SessionLeaseInfo{PID: os.Getpid(), WriterID: agent.SessionWriterID()},
	}

	lease, err := reclaimOwnSessionLease(path, held)
	if err != nil {
		t.Fatalf("reclaimOwnSessionLease: %v", err)
	}
	defer lease.Release()
	if lease.Path() != agent.CanonicalSessionPath(path) {
		t.Fatalf("reclaimed %q, want %q", lease.Path(), agent.CanonicalSessionPath(path))
	}
}

// A lease whose info names someone else is theirs. Reclaim would refuse it at
// the lock anyway; refusing here keeps the intent explicit.
func TestReclaimOwnSessionLeaseLeavesAnotherProcessAlone(t *testing.T) {
	path := leasePath(t, "foreign")
	held := &agent.SessionLeaseError{
		Path: path,
		Info: &agent.SessionLeaseInfo{PID: os.Getpid() + 1, WriterID: "someone-else"},
	}

	if _, err := reclaimOwnSessionLease(path, held); !errors.Is(err, held) {
		t.Fatalf("reclaimOwnSessionLease returned %v, want the original refusal", err)
	}
}

// Damaged metadata with a free lock is a leftover, not an owner: refusing on a
// missing info file would wedge a session nobody holds as permanently busy.
func TestReclaimOwnSessionLeaseProceedsWithoutInfo(t *testing.T) {
	path := leasePath(t, "noinfo")
	lease, err := reclaimOwnSessionLease(path, &agent.SessionLeaseError{Path: path})
	if err != nil {
		t.Fatalf("reclaimOwnSessionLease with no info: %v", err)
	}
	lease.Release()
}

func TestReclaimOwnSessionLeaseIgnoresUnrelatedFailures(t *testing.T) {
	path := leasePath(t, "other")
	cause := errors.New("disk on fire")
	if _, err := reclaimOwnSessionLease(path, cause); !errors.Is(err, cause) {
		t.Fatalf("reclaimOwnSessionLease returned %v, want the original error", err)
	}
}

// The safety the fix must not spend: a live holder keeps the OS lock, so a
// second keeper still loses. Recovering our own leftover and stealing from a
// live sibling look identical from the owner map and are told apart by the lock.
func TestRebindStillRefusesALiveHolderInThisProcess(t *testing.T) {
	path := leasePath(t, "live")
	holder := NewSessionLeaseKeeper()
	if err := holder.Rebind(path); err != nil {
		t.Fatalf("first Rebind: %v", err)
	}
	defer holder.Release()

	other := NewSessionLeaseKeeper()
	if err := other.Rebind(path); err == nil {
		other.Release()
		t.Fatal("a second keeper took a lease the first one is still holding")
	} else if !errors.Is(err, agent.ErrSessionLeaseHeld) {
		t.Fatalf("second Rebind failed with %v, want a held-lease refusal", err)
	}
	if holder.HeldPath() != agent.CanonicalSessionPath(path) {
		t.Fatalf("holder lost its lease to the refused bind: %q", holder.HeldPath())
	}
}

func TestRebindToTheHeldPathIsANoOp(t *testing.T) {
	path := leasePath(t, "same")
	keeper := NewSessionLeaseKeeper()
	if err := keeper.Rebind(path); err != nil {
		t.Fatalf("first Rebind: %v", err)
	}
	defer keeper.Release()
	if err := keeper.Rebind(path); err != nil {
		t.Fatalf("rebinding to the held path: %v", err)
	}
}

// "Another Reasonix process (pid <ours>)" reads as a second window to close,
// which is what made the report unactionable.
func TestSessionInUseMessageDoesNotCallThisProcessAnother(t *testing.T) {
	held := &agent.SessionLeaseError{
		Path: leasePath(t, "self"),
		Info: &agent.SessionLeaseInfo{PID: os.Getpid(), WriterID: agent.SessionWriterID()},
	}
	msg := SessionInUseMessage(held)
	if strings.Contains(msg, "another Reasonix process") {
		t.Fatalf("SessionInUseMessage = %q, want it not to blame a separate process", msg)
	}
	if !strings.Contains(msg, "this Reasonix") {
		t.Fatalf("SessionInUseMessage = %q, want it to say the holder is this Reasonix", msg)
	}
}

func TestSessionInUseMessageStillNamesAForeignHolder(t *testing.T) {
	held := &agent.SessionLeaseError{
		Path: leasePath(t, "far"),
		Info: &agent.SessionLeaseInfo{PID: os.Getpid() + 1, Hostname: "DESKTOP-X"},
	}
	msg := SessionInUseMessage(held)
	if !strings.Contains(msg, "another Reasonix process") || !strings.Contains(msg, "DESKTOP-X") {
		t.Fatalf("SessionInUseMessage = %q, want the foreign holder named", msg)
	}
}
