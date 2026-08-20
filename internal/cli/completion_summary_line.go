// Rendering for the end-of-turn completion summary: the one line that says what
// the turn did to the checks it was measured against.
package cli

import (
	"fmt"
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
)

// formatCompletionSummaryLine renders a content-free quality summary for TUI scrollback.
func formatCompletionSummaryLine(c *event.CompletionSummaryInfo) string {
	if c == nil {
		return ""
	}
	preset := strings.TrimSpace(c.Preset)
	if preset == "" {
		preset = "balanced"
	}
	line := fmt.Sprintf("%s · %s · mut=%d · checks %d✓/%d✗/%d⊘",
		preset, c.Verdict, c.Mutations, c.ChecksPassed, c.ChecksFailed, c.ChecksSuppressed)
	if c.Review != "" && c.Review != "none" {
		line += " · review=" + c.Review
	}
	if len(c.GapKinds) > 0 {
		line += " · gaps=" + strings.Join(c.GapKinds, ",")
	}
	// A rewritten check is why the pass count above may not mean what it did
	// last turn, so it sits on the same line as that count.
	if len(c.CriteriaRewritten) > 0 {
		line += " · criteria-rewritten=" + strings.Join(c.CriteriaRewritten, ",")
	}
	return line
}

func completionSummaryWarning(c *event.CompletionSummaryInfo) string {
	if c.Blocked() {
		return i18n.M.CompletionSummaryBlocked
	}
	return i18n.M.CompletionSummaryNeedsAttention
}
