package cli

import (
	"fmt"
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
)

// compactionCardLines renders a finished compaction as a titled card: a header
// with the message count and trigger, then the structured summary under a dim
// gutter so it reads as one block in scrollback. The summary is also the new
// context base, so this card is the user's window into exactly what was kept.
func compactionCardLines(c event.Compaction) []string {
	trigger := c.Trigger
	switch c.Trigger {
	case "auto":
		trigger = i18n.M.CompactionAuto
	case "manual":
		trigger = i18n.M.CompactionManual
	}
	header := fmt.Sprintf("%s · %d %s · %s", i18n.M.CompactionTitle, c.Messages, i18n.M.CompactionUnit, trigger)
	lines := []string{accent("◆ " + header)}
	if quality := compactionQualityLine(c); quality != "" {
		lines = append(lines, dim("  │ "+quality))
	}
	for ln := range strings.SplitSeq(strings.TrimRight(c.Summary, "\n"), "\n") {
		lines = append(lines, dim("  │ "+ln))
	}
	if c.Archive != "" {
		lines = append(lines, dim("  │ archived "+c.Archive))
	}
	return lines
}

// compactionQualityLine says what the fold cost and what the digest kept of it.
// A digest reads as complete whatever it dropped, so the count of changes it
// carried is what a reader cannot get from the text itself.
func compactionQualityLine(c event.Compaction) string {
	var parts []string
	if c.SourceTokens > 0 && c.ProjectionTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s → %s", shortTokens(c.SourceTokens), shortTokens(c.ProjectionTokens)))
	}
	if c.CoverageRequired > 0 {
		kept := fmt.Sprintf("%d/%d %s", c.CoverageRequired-c.CoverageMissing, c.CoverageRequired, i18n.M.CompactionChangesKept)
		if c.CoverageRepaired {
			kept += " (" + i18n.M.CompactionRepaired + ")"
		}
		parts = append(parts, kept)
	}
	return strings.Join(parts, " · ")
}
