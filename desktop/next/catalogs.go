package main

import (
	"context"
	"time"

	"reasonix/internal/history"
)

// The history projection is process-wide, shared by every pane, so it is closed
// at the process boundary and not with any one pane's controller. Draining
// queued index writes first is what saves the next launch a full re-scan of
// every root.
func closeProcessCatalogs() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = history.FlushSharedCatalog(ctx)
	_ = history.CloseSharedCatalog(ctx)
}
