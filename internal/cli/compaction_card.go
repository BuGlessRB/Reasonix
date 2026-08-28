package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

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
	return lines
}

// handleCompaction owns the whole fold as one thing: a live line while the
// digest is written, replaced by the card once it is.
func (m *chatTUI) handleCompaction(e event.Event) {
	switch e.Kind {
	case event.CompactionStarted:
		m.finalizeStreamed()
		m.compactionLineIdx = len(m.transcript)
		m.compactionTail = ""
		m.commitLine(dim("  ⋯ " + i18n.M.CompactionWorking))
	case event.CompactionProgress:
		m.streamCompaction(e.Text)
	case event.CompactionDone:
		m.compactionLineIdx = -1
		// An aborted pass carries no summary; the accompanying Notice (auto) or
		// compactDoneMsg error (manual) explains why, so don't draw an empty card.
		if e.Compaction.Summary == "" {
			return
		}
		m.finalizeStreamed()
		for _, ln := range compactionCardLines(e.Compaction) {
			m.commitLine(ln)
		}
	}
}

// streamCompaction shows the digest being written on the "compacting…" line
// itself. A fold can take a minute, and a placeholder that says nothing for a
// minute cannot be told apart from one that has hung. Only the tail is shown:
// a digest is thousands of tokens, and printing it would bury the transcript
// under a summary the finished card is about to render anyway.
func (m *chatTUI) streamCompaction(chunk string) {
	if m.compactionLineIdx < 0 || chunk == "" {
		return
	}
	m.compactionTail = lastCompactionLine(m.compactionTail + chunk)
	line := "  ⋯ " + i18n.M.CompactionWorking
	if m.compactionTail != "" {
		line += " · " + m.compactionTail
	}
	m.setTranscriptBlock(m.compactionLineIdx, dim(ansi.Truncate(line, max(m.width-2, 24), "…")), transcriptSource{kind: transcriptSourceFixed})
}

// lastCompactionLine keeps the newest line of the digest so far, bounded, so
// the retained tail cannot grow with the summary.
func lastCompactionLine(text string) string {
	if i := strings.LastIndexByte(strings.TrimRight(text, "\n"), '\n'); i >= 0 {
		text = text[i+1:]
	}
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > 120 {
		text = string(r[len(r)-120:])
	}
	return text
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
		switch {
		case c.CoverageRepaired:
			kept += " (" + i18n.M.CompactionRepaired + ")"
		case c.CoverageBackstopped:
			// The facts are in the projection, just not because the digest
			// carried them. Saying so keeps the ratio honest either way.
			kept += " (" + i18n.M.CompactionBackstopped + ")"
		}
		parts = append(parts, kept)
	}
	return strings.Join(parts, " · ")
}
