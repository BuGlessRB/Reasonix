package main

import "reasonix/internal/config"

// rebindEffortModel commits the model/effort pair under the caller's tab lock.
func (tab *WorkspaceTab) rebindEffortModel(cfg *config.Config, model string) {
	tab.effort = config.RebindSessionEffort(cfg, tab.model, model, tab.effort)
	tab.model = model
}
