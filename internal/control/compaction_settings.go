package control

import (
	"fmt"

	"reasonix/internal/agent"
	"reasonix/internal/config"
)

// CompactionSettings is what the two configured bounds are and which of them
// the session is actually running under. The pair is the point: they are set
// independently, only the lower one ever fires, and a panel showing the
// settings alone cannot say which — a 1M window declared against the default
// soft limit folds at 160k, and nothing on that screen would explain it.
type CompactionSettings struct {
	// SoftLimitTokens is the stored value, not the resolved one: zero means
	// the default and a negative value means off, and a caller that cannot
	// tell those apart cannot offer them.
	SoftLimitTokens  int     `json:"soft_limit_tokens"`
	DefaultSoftLimit int     `json:"default_soft_limit"`
	Ratio            float64 `json:"ratio"`
	// ContextWindow is the window in force, zero when nobody declared one —
	// which is also what turns automatic maintenance off entirely.
	ContextWindow int `json:"context_window"`
	// Trigger is the boundary this session folds at, read from the agent
	// rather than recomputed here: two implementations of one rule is how the
	// number a user reads stops matching the one that fires.
	Trigger int    `json:"trigger"`
	Path    string `json:"path"`
}

// CompactionSettings reports the fold bounds for this session.
func (c *Controller) CompactionSettings() CompactionSettings {
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	out := CompactionSettings{
		SoftLimitTokens:  cfg.Agent.ContextSoftLimitTokens,
		DefaultSoftLimit: agent.DefaultContextSoftLimitTokens,
		Ratio:            c.CompactRatio(),
		Path:             path,
	}
	if c.executor != nil {
		out.ContextWindow = c.executor.ContextWindow()
		out.Trigger = c.executor.CompactTrigger()
	}
	return out
}

// SaveCompactionSettings persists the economic boundary. The caller rebuilds:
// boot binds these bounds into every agent it assembles, parent and sub-agent
// alike, so a live session keeps the boundary it was built with until it is
// replaced.
func (c *Controller) SaveCompactionSettings(softLimitTokens int) error {
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	if err := cfg.SetContextSoftLimitTokens(softLimitTokens); err != nil {
		return fmt.Errorf("context soft limit: %w", err)
	}
	return cfg.SaveTo(path)
}
