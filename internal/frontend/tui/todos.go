package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/frontend/termrender"
)

const todoRows = 8

type todosMsg struct {
	items []TodoItem
	err   error
}

func (m *model) fetchTodos() tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.Todos(m.ctx)
		return todosMsg{items: items, err: err}
	}
}

// todoLines draws the kernel's task list while it has work left. A list
// that ran to the end is spent, and keeping it up would read as the next
// turn already having a plan.
func (m *model) todoLines() []string {
	open := false
	for _, t := range m.todos {
		if t.Status != "completed" {
			open = true
		}
	}
	if !open {
		return nil
	}
	lines := []string{termrender.Dim("  Tasks")}
	for i, t := range m.todos {
		if i == todoRows {
			lines = append(lines, termrender.Dim("    …"))
			break
		}
		indent := "    " + strings.Repeat("  ", max(t.Level, 0))
		switch t.Status {
		case "completed":
			lines = append(lines, termrender.Dim(indent+"✓ "+oneLine(t.Content, m.width-10)))
		case "in_progress":
			label := t.Content
			if t.ActiveForm != "" {
				label = t.ActiveForm
			}
			lines = append(lines, termrender.Accent(indent+"▸ ")+termrender.Bold(oneLine(label, m.width-10)))
		default:
			lines = append(lines, indent+"○ "+oneLine(t.Content, m.width-10))
		}
	}
	return lines
}
