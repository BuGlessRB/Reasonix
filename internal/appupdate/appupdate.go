// Package appupdate is where owning the Studio application meets replacing it.
package appupdate

import (
	"context"

	"reasonix/internal/repair"
)

// ApplicationOwner owns the running desktop application: it can bring it to a
// state where its own files may be replaced, and it knows how to start what
// takes its place. Nothing here names a shell, and whether the two halves
// belong to one process is a platform's answer — a waiting helper starts the
// successor on Windows and macOS, the departing application does on Linux.
type ApplicationOwner interface {
	// PrepareForUpdate releases what would keep the application's own files
	// from being replaced.
	PrepareForUpdate(ctx context.Context) error
	// RelaunchAfterUpdate starts what replaced it.
	RelaunchAfterUpdate(ctx context.Context) error
}

// Capability is what a hub serves updates through. It is declared here rather
// than taken from serve because a kernel package may not import a frontend, and
// Go's interfaces meet structurally: the hub's port is satisfied without the
// two packages naming each other.
type Capability interface {
	AcknowledgeLaunchHealth() error
}

// New returns the capability a hub serves updates through, and nil where
// nothing owns the application. That nil is the gate: a kernel with no owner
// registers no update routes at all, so `reasonix serve` does not acquire the
// power to replace a desktop application by sharing an engine with one. The
// return type is an interface so the nil survives the assignment.
func New(owner ApplicationOwner, runningVersion string) Capability {
	if owner == nil {
		return nil
	}
	// Read now, before a concurrent update can rewrite it: what this launch may
	// retire is the transaction it booted from, and nothing later can tell that
	// apart from one written since.
	return &capability{
		owner:   owner,
		running: runningVersion,
		witness: repair.CaptureUpdateHealth(runningVersion),
	}
}

type capability struct {
	owner   ApplicationOwner
	running string
	witness *repair.UpdateHealthWitness
}

// AcknowledgeLaunchHealth retires the update this launch booted from, and only
// that one. The caller passes nothing because it may name nothing: the swap is
// performed by a process that cannot judge it, and a shell that could hand over
// a transaction id could commit somebody else's. A launch that did not boot
// from an update has nothing to retire and says so by succeeding.
func (c *capability) AcknowledgeLaunchHealth() error {
	return c.witness.Acknowledge(c.running)
}
