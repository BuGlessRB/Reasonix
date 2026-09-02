// edit_compaction.go — the two bounds that decide when a session folds. They
// are set independently and only the lower one fires, so they are edited in one
// place rather than beside unrelated settings.
package config

import (
	"fmt"
	"math"
)

// SetCompactRatio updates the sole automatic compaction threshold. Presets are
// 0.70 / 0.80 / 0.85; any fraction of the window is allowed.
func (c *Config) SetCompactRatio(ratio float64) error {
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 || ratio >= 1 {
		return fmt.Errorf("compact ratio %v: must be a fraction of the window, above 0 and below 1", ratio)
	}
	c.Agent.CompactRatio = ratio
	return nil
}

// SetContextSoftLimitTokens updates the economic maintenance boundary: the
// visible input size a fold happens at whatever the declared window allows.
// Zero restores the default and a negative value turns the boundary off,
// because both are answers a caller has to be able to give.
func (c *Config) SetContextSoftLimitTokens(tokens int) error {
	c.Agent.ContextSoftLimitTokens = tokens
	return nil
}
