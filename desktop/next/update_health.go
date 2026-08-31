package main

import (
	"context"
	"log/slog"
	"time"

	"reasonix/internal/repair"
)

// What this file owns is when the acknowledgement happens, so both halves of
// that are seams: a test has to watch the timing without a real transaction.
var (
	// updateProbation is how long a loaded window must survive before the update
	// it booted from is committed. A build that paints once and then dies is not
	// evidence that the replacement works.
	updateProbation    = 2 * time.Second
	commitUpdateHealth = func(w *repair.UpdateHealthWitness, running string) error {
		return w.Acknowledge(running)
	}
)

// acknowledgeUpdateHealth retires the update this launch booted from. The
// updater performed the swap and cannot judge it; this process can, and only
// after a renderer loaded into a window that stayed up. A launch that shuts
// down inside probation commits nothing and leaves the rollback material.
func (a *App) acknowledgeUpdateHealth(ctx context.Context) {
	witness := a.updateHealth
	if witness == nil {
		return
	}
	go func() {
		timer := time.NewTimer(updateProbation)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		if err := commitUpdateHealth(witness, version); err != nil {
			slog.Warn("studio: commit healthy update", "err", err)
		}
	}()
}
