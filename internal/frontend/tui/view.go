package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"reasonix/internal/frontend/termrender"
)

// View draws the live region: what is still changing, the composer and the
// status line. Everything settled is already in the terminal's scrollback.
func (m *model) View() tea.View {
	live := append(append(m.liveLines(), m.todoLines()...), m.menuLines()...)
	composer := m.composerLines()
	status := m.statusLine()
	// The live region is redrawn in place, so it must stay well inside the
	// screen: a frame taller than the screen scrolls, and the rows that
	// scrolled off can no longer be cleared before the next print.
	room := max(min(m.height/2, m.height-len(composer)-2), 3)
	if len(live) > room {
		live = live[len(live)-room:]
	}
	lines := make([]string, 0, len(live)+len(composer)+2)
	lines = append(lines, live...)
	above := len(lines)
	lines = append(lines, composer...)
	lines = append(lines, status)
	// A row as wide as the terminal wraps on its own, which adds a row the
	// renderer does not know it drew.
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, max(m.width-1, 1), "")
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	if c := m.composer.Cursor(); c != nil && (m.tr.OpenPrompt() == nil || m.tr.OpenPrompt().Kind == ItemAsk) {
		c.X += 2
		c.Y += above
		v.Cursor = c
	}
	return v
}

func (m *model) liveLines() []string {
	var out []string
	for i := range m.tr.Items {
		it := &m.tr.Items[i]
		if m.committed[it.ID] {
			continue
		}
		switch {
		case it.Kind == ItemUser && it.Pending:
			out = append(out, termrender.Dim("  ⧗ "+oneLine(it.Text, m.width-6)))
		case it.Kind == ItemSay:
			shown := m.sayShown[it.ID]
			if rest := it.Text[min(shown, len(it.Text)):]; rest != "" {
				out = append(out, strings.Split(renderSayPart(rest, shown == 0, m.width), "\n")...)
			} else if it.Reasoning != "" && !it.Done {
				out = append(out, termrender.Dim("  ✻ thinking…"))
			}
		case it.Kind == ItemApproval && it.Verdict == "":
			out = append(out, approvalCard(it, m.width)...)
		case it.Kind == ItemAsk && it.Verdict == "":
			out = append(out, m.askCard(it)...)
		default:
			if r := renderItem(it, m.width, 0); r != "" {
				out = append(out, strings.Split(r, "\n")...)
			}
		}
	}
	if m.tr.Running && len(out) == 0 {
		out = append(out, termrender.Dim("  ✻ working… (esc to interrupt)"))
	}
	return out
}

func approvalCard(it *Item, width int) []string {
	a := it.Approval
	if a.Kind == "plan" {
		return []string{
			termrender.Accent("  ◇ ") + termrender.Bold("Run this plan?"),
			termrender.Dim("    y run it · n revise · x leave plan mode"),
		}
	}
	lines := []string{termrender.Accent("  ◇ ") + termrender.Bold("Allow "+a.Tool+"?")}
	if a.Subject != "" {
		lines = append(lines, "    "+oneLine(a.Subject, width-6))
	}
	if a.Reason != "" {
		lines = append(lines, termrender.Dim("    "+oneLine(a.Reason, width-6)))
	}
	keys := "y once"
	if a.AllowsSession {
		keys += " · a this session"
	}
	if a.AllowsPersist {
		keys += " · p always"
	}
	return append(lines, termrender.Dim("    "+keys+" · n deny"))
}

func (m *model) composerLines() []string {
	mark := termrender.Accent("› ")
	if m.shell {
		mark = termrender.Yellow("! ")
	}
	rows := strings.Split(m.composer.View(), "\n")
	for i := range rows {
		if i == 0 {
			rows[i] = mark + rows[i]
		} else {
			rows[i] = "  " + rows[i]
		}
	}
	return rows
}

func (m *model) statusLine() string {
	s := m.status
	parts := []string{}
	if s.ModelRef != "" {
		parts = append(parts, s.ModelRef)
	}
	if s.ToolApprovalMode != "" {
		parts = append(parts, s.ToolApprovalMode)
	}
	if s.Window > 0 {
		parts = append(parts, "ctx "+tokens(s.Used)+"/"+tokens(s.Window))
	}
	hint := "shift+tab mode · ctrl+j newline"
	switch {
	case !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) < quitArmWindow:
		hint = "ctrl+c again to exit"
	case m.tr.Running:
		hint = "esc interrupt · enter queue · ctrl+s steer"
	case m.shell:
		hint = "shell mode · esc to leave"
	}
	return termrender.Dim("  " + strings.Join(append(parts, hint), " · "))
}

func tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	}
	return fmt.Sprint(n)
}
