package cli

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/memory"
)

// showContextReport prints the window, the thresholds derived from it, and how
// the last maintenance pass ended. It commits a block rather than a notice
// because the numbers are meant to be compared with each other. "fold" edits
// the economic bound here rather than as its own command, because the answer
// it changes is the one this report already gives.
func (m *chatTUI) showContextReport(input string) tea.Cmd {
	m.echoLocalCommand(input)
	if rest, ok := foldArgument(input); ok {
		return m.setFoldThreshold(rest)
	}
	summary, detail := m.ctrl.ContextReport()
	if detail != "" {
		summary += "\n" + detail
	}
	m.commitLine(summary + "\n" + m.foldBounds())
	return nil
}

func foldArgument(input string) (string, bool) {
	fields := strings.Fields(input)
	if len(fields) >= 2 && fields[1] == "fold" {
		return strings.Join(fields[2:], " "), true
	}
	return "", false
}

func (m *chatTUI) setFoldThreshold(arg string) tea.Cmd {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		m.commitLine(m.foldBounds())
		return nil
	}
	tokens, err := strconv.Atoi(arg)
	if err != nil {
		m.notice("fold: " + arg + " is not a whole number of tokens")
		return nil
	}
	if err := m.ctrl.SaveCompactionSettings(tokens); err != nil {
		m.notice("fold: " + err.Error())
		return nil
	}
	m.commitLine(m.foldBounds())
	return nil
}

// foldBounds says both bounds and which one is current, because only the lower
// one fires: a 1M window against the default threshold folds at 160k, and the
// settings alone do not say so.
func (m *chatTUI) foldBounds() string {
	c := m.ctrl.CompactionSettings()
	var b strings.Builder
	switch {
	case c.SoftLimitTokens < 0:
		b.WriteString("fold threshold  off\n")
	case c.SoftLimitTokens == 0:
		fmt.Fprintf(&b, "fold threshold  %d (default)\n", c.DefaultSoftLimit)
	default:
		fmt.Fprintf(&b, "fold threshold  %d\n", c.SoftLimitTokens)
	}
	fmt.Fprintf(&b, "window share    %d x %.2f\n", c.ContextWindow, c.Ratio)
	if c.ContextWindow == 0 {
		b.WriteString("folds at        never (no window declared)")
		return b.String()
	}
	fmt.Fprintf(&b, "folds at        %d", c.Trigger)
	return b.String()
}

func (m *chatTUI) rememberNote(note string) {
	if note == "" {
		m.notice("nothing to remember")
		return
	}
	path, err := m.ctrl.QuickAdd(memory.ScopeProject, note)
	if err != nil {
		m.notice("memory: " + err.Error())
		return
	}
	m.notice("remembered → " + path)
}
