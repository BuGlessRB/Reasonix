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
	room := max(min(m.height/2, m.height-len(composer)-4), 3)
	if len(live) > room {
		live = live[len(live)-room:]
	}
	lines := make([]string, 0, len(live)+len(composer)+3)
	lines = append(lines, live...)
	lines = append(lines, m.rule())
	above := len(lines)
	lines = append(lines, composer...)
	lines = append(lines, m.rule(), status)
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
	if m.tr.Running && m.tr.OpenPrompt() == nil {
		out = append(out, "", m.spinnerLine())
	}
	return out
}

func approvalCard(it *Item, width int) []string {
	a := it.Approval
	if a.Kind == "plan" {
		return card([]string{
			termrender.Accent("◇ ") + termrender.Bold("Run this plan?"),
			"",
			keyHints("y", "run it", "n", "revise", "x", "leave plan mode"),
		}, width, termrender.Accent)
	}
	lines := []string{termrender.Yellow("◇ ") + termrender.Bold("Allow "+termrender.ToolDisplayName(a.Tool)+"?")}
	if a.Subject != "" {
		lines = append(lines, "  "+oneLine(a.Subject, width-10))
	}
	if a.Reason != "" {
		lines = append(lines, termrender.Dim("  "+oneLine(a.Reason, width-10)))
	}
	keys := []string{"y", "once"}
	if a.AllowsSession {
		keys = append(keys, "a", "this session")
	}
	if a.AllowsPersist {
		keys = append(keys, "p", "always")
	}
	lines = append(lines, "", keyHints(append(keys, "n", "deny")...))
	return card(lines, width, termrender.Yellow)
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

// statusLine keeps the session's posture on the left and the keys that
// matter right now on the right, dropping the keys first when space is short.
func (m *model) statusLine() string {
	s := m.status
	left := []string{}
	if s.ToolApprovalMode != "" {
		left = append(left, modeBadge(s.ToolApprovalMode))
	}
	if s.ModelRef != "" {
		model := modelName(s.ModelRef)
		if s.Effort != "" {
			model += termrender.Dim(" · " + s.Effort)
		}
		left = append(left, model)
	}
	if s.Window > 0 {
		left = append(left, gauge(s.Used, s.Window))
	}
	var right string
	switch {
	case !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) < quitArmWindow:
		right = termrender.Yellow("ctrl+c again to exit")
	case m.tr.Running:
		right = keyHints("esc", "interrupt", "enter", "queue", "ctrl+s", "steer")
	case m.shell:
		right = keyHints("esc", "leave shell")
	default:
		right = keyHints("shift+tab", "mode", "ctrl+j", "newline")
	}
	line := "  " + strings.Join(left, "   ")
	gap := m.width - 1 - termrender.VisibleWidth(line) - termrender.VisibleWidth(right) - 1
	if gap < 2 {
		return line
	}
	return line + strings.Repeat(" ", gap) + right
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
