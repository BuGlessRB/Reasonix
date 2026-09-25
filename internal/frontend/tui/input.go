package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A paste this large stands in the composer as one token, the way the reader
// would describe it, and goes to the model whole.
const (
	pasteFoldChars = 800
	pasteFoldLines = 3
)

var pasteToken = regexp.MustCompile(`\[Pasted text #(\d+) \+\d+ lines\]`)

type pasteStore struct {
	next  int
	texts map[int]string
}

// fold returns what the composer shows for a paste: the text itself, or a
// token for it when it would bury the line being written.
func (p *pasteStore) fold(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Count(text, "\n") + 1
	if len(text) < pasteFoldChars && lines <= pasteFoldLines {
		return text
	}
	if p.texts == nil {
		p.texts = map[int]string{}
	}
	p.next++
	p.texts[p.next] = text
	return fmt.Sprintf("[Pasted text #%d +%d lines]", p.next, lines)
}

// expand replaces each paste token with the text it stands for.
func (p *pasteStore) expand(s string) string {
	return pasteToken.ReplaceAllStringFunc(s, func(tok string) string {
		n, _ := strconv.Atoi(pasteToken.FindStringSubmatch(tok)[1])
		if text, ok := p.texts[n]; ok {
			return text
		}
		return tok
	})
}

func (m *model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if open := m.tr.OpenPrompt(); open != nil && open.Kind == ItemApproval {
		if cmd, handled := m.answerApproval(open, msg.String()); handled {
			return m, cmd
		}
	}
	empty := m.composer.Value() == ""
	switch msg.String() {
	case "enter":
		return m, m.send(false)
	case "ctrl+s":
		return m, m.send(true)
	case "esc":
		if m.tr.Running {
			return m, m.call("cancel", m.client.Cancel)
		}
		if m.shell && empty {
			m.shell = false
		}
		return m, nil
	case "ctrl+c":
		switch {
		case m.tr.Running:
			return m, m.call("cancel", m.client.Cancel)
		case !empty:
			m.composer.Reset()
			return m, nil
		case time.Since(m.quitArmedAt) < quitArmWindow:
			return m, tea.Quit
		}
		m.quitArmedAt = time.Now()
		return m, nil
	case "ctrl+d":
		if empty && !m.tr.Running {
			return m, tea.Quit
		}
	case "shift+tab":
		return m, m.cycleApprovalMode()
	case "backspace":
		if m.shell && empty {
			m.shell = false
			return m, nil
		}
	case "!":
		if empty && !m.shell {
			m.shell = true
			return m, nil
		}
	case "up", "down":
		if m.composer.LineCount() <= 1 && m.recall(msg.String() == "up") {
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

// send hands the composer's text to the kernel. Idle, it starts a turn (or
// runs the `!` command); while a turn runs, it queues: steer lands at the
// turn's next tool boundary, a follow-up once the turn is done.
func (m *model) send(steer bool) tea.Cmd {
	display := strings.TrimSpace(m.composer.Value())
	if display == "" {
		return nil
	}
	text := m.pastes.expand(display)
	m.history = append(m.history, display)
	m.histAt = len(m.history)
	m.composer.Reset()
	if m.shell {
		m.shell = false
		display, text = "! "+display, "!"+text
		if m.tr.Running {
			m.tr.AddNotice("warn", "a shell command runs between turns; wait for this one to finish")
			return m.commit()
		}
	}
	if m.tr.Running {
		row := m.tr.AddQueued(display, steer)
		return func() tea.Msg {
			id, err := m.client.Queue(m.ctx, text, steer)
			return queuedMsg{row: row, itemID: id, err: err}
		}
	}
	m.tr.AddUser(display)
	return tea.Batch(m.commit(), m.call("send", func(ctx context.Context) error { return m.client.Submit(ctx, text) }))
}

// recall walks the composer through what was sent in this session.
func (m *model) recall(back bool) bool {
	if len(m.history) == 0 {
		return false
	}
	if back {
		m.histAt = max(m.histAt-1, 0)
	} else {
		m.histAt = min(m.histAt+1, len(m.history))
	}
	if m.histAt == len(m.history) {
		m.composer.Reset()
	} else {
		m.composer.SetValue(m.history[m.histAt])
	}
	return true
}

var approvalModes = []string{"ask", "auto"}

func (m *model) cycleApprovalMode() tea.Cmd {
	next := approvalModes[0]
	for i, mode := range approvalModes {
		if mode == m.status.ToolApprovalMode {
			next = approvalModes[(i+1)%len(approvalModes)]
		}
	}
	m.status.ToolApprovalMode = next
	return tea.Sequence(m.call("mode", func(ctx context.Context) error { return m.client.SetApprovalMode(ctx, next) }), m.fetchStatus())
}

// answerApproval takes the single-key answers an approval card offers. Only
// the answers the host said it will honour are accepted.
func (m *model) answerApproval(it *Item, k string) (tea.Cmd, bool) {
	a := it.Approval
	if a.Kind == "plan" {
		return m.answerPlan(it, k)
	}
	var verdict string
	var allow, session, persist bool
	switch k {
	case "y":
		verdict, allow = "once", true
	case "a":
		if !a.AllowsSession {
			return nil, true
		}
		verdict, allow, session = "session", true, true
	case "p":
		if !a.AllowsPersist {
			return nil, true
		}
		verdict, allow, persist = "always", true, true
	case "n", "esc":
		verdict = "deny"
	default:
		return nil, false
	}
	m.tr.Decide(it.ID, verdict)
	id := a.ID
	return tea.Batch(m.commit(), m.call("approve", func(ctx context.Context) error {
		return m.client.Approve(ctx, id, allow, session, persist)
	})), true
}

// planActions are the plan card's three endings: run it, send it back for a
// revision, or leave plan mode without running it.
var planActions = map[string]string{"y": "start_execution", "n": "revise_plan", "esc": "revise_plan", "x": "exit_plan"}

func (m *model) answerPlan(it *Item, k string) (tea.Cmd, bool) {
	action, ok := planActions[k]
	if !ok {
		return nil, false
	}
	m.tr.Decide(it.ID, action)
	id := it.Approval.ID
	return tea.Batch(m.commit(), m.call("plan", func(ctx context.Context) error {
		err := m.client.PlanDecision(ctx, id, action)
		if Code(err) == CodePlanStale {
			return nil
		}
		return err
	})), true
}
