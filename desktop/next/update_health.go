package main

import (
	"context"
	"log/slog"
	"time"

	"reasonix/internal/serve"
)

// What this file owns is when the acknowledgement happens, so both halves of
// that are seams: a test has to watch the timing without a real transaction.
var (
	// updateProbation is how long a loaded window must survive before the update
	// it booted from is committed. A build that paints once and then dies is not
	// evidence that the replacement works.
	updateProbation    = 2 * time.Second
	commitUpdateHealth = func(h serve.UpdateHost) error { return h.AcknowledgeLaunchHealth() }
)

// acknowledgeUpdateHealth reports that this launch is working. The updater
// performed the swap and cannot judge it; this process can, and only after a
// renderer loaded into a window that stayed up. The window supplies the timing
// and names nothing: what it retires is the transaction the host read at
// startup. Shutting down inside probation leaves the rollback material.
func (a *App) acknowledgeUpdateHealth(ctx context.Context) {
	host := a.updateHost
	if host == nil {
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
		if err := commitUpdateHealth(host); err != nil {
			slog.Warn("studio: commit healthy update", "err", err)
		}
	}()
}
