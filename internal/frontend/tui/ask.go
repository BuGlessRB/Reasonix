package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/frontend/termrender"
)

// askState walks an open question card one question at a time. picks holds
// each question's answer so far: option labels, plus typed text, which the
// kernel takes as an answer no option offered.
type askState struct {
	item  int
	at    int
	picks [][]string
}

func (m *model) openAsk(it *Item) *askState {
	if m.ask == nil || m.ask.item != it.ID {
		m.ask = &askState{item: it.ID, picks: make([][]string, len(it.Ask.Questions))}
		for i, q := range it.Ask.Questions {
			m.ask.picks[i] = slices.Clone(q.Default)
		}
	}
	return m.ask
}

// answerAsk takes a key while a question card is open. A digit picks an
// option: on a single-choice question that answers it, on a multi-choice one
// it toggles. Enter answers with the typed text, or, with nothing typed,
// confirms what is picked. Esc answers with nothing, which is the kernel's
// way of hearing "no answer".
func (m *model) answerAsk(it *Item, k string) (tea.Cmd, bool) {
	st := m.openAsk(it)
	q := it.Ask.Questions[st.at]
	switch {
	case k == "esc":
		for i := range st.picks {
			st.picks[i] = nil
		}
		return m.sendAsk(it), true
	case len(k) == 1 && k[0] >= '1' && k[0] <= '9' && m.composer.Value() == "":
		n := int(k[0] - '1')
		if n >= len(q.Options) {
			return nil, true
		}
		label := q.Options[n].Label
		if !q.Multi {
			st.picks[st.at] = []string{label}
			return m.nextQuestion(it), true
		}
		if i := slices.Index(st.picks[st.at], label); i >= 0 {
			st.picks[st.at] = slices.Delete(st.picks[st.at], i, i+1)
		} else {
			st.picks[st.at] = append(st.picks[st.at], label)
		}
		return nil, true
	case k == "enter":
		if typed := strings.TrimSpace(m.composer.Value()); typed != "" {
			m.composer.Reset()
			st.picks[st.at] = append(st.picks[st.at], typed)
		}
		if len(st.picks[st.at]) == 0 {
			return nil, true
		}
		return m.nextQuestion(it), true
	}
	return nil, false
}

func (m *model) nextQuestion(it *Item) tea.Cmd {
	st := m.ask
	if st.at+1 < len(it.Ask.Questions) {
		st.at++
		return nil
	}
	return m.sendAsk(it)
}

// sendAsk answers the card. The verdict it seals the card with is what was
// answered, so the line left in the scrollback says it.
func (m *model) sendAsk(it *Item) tea.Cmd {
	answers := make([]AskAnswer, len(it.Ask.Questions))
	said := make([]string, 0, len(it.Ask.Questions))
	for i, q := range it.Ask.Questions {
		answers[i] = AskAnswer{QuestionID: q.ID, Selected: m.ask.picks[i]}
		if len(m.ask.picks[i]) > 0 {
			said = append(said, strings.Join(m.ask.picks[i], ", "))
		}
	}
	verdict := "declined"
	if len(said) > 0 {
		verdict = strings.Join(said, " · ")
	}
	m.ask = nil
	m.tr.Decide(it.ID, verdict)
	id := it.Ask.ID
	return tea.Batch(m.commit(), m.call("answer", func(ctx context.Context) error { return m.client.Answer(ctx, id, answers) }))
}

// askCard draws the question in hand, with what is picked so far.
func (m *model) askCard(it *Item) []string {
	st := m.openAsk(it)
	qs := it.Ask.Questions
	if len(qs) == 0 {
		return nil
	}
	q := qs[st.at]
	title := "A question for you"
	if len(qs) > 1 {
		title = fmt.Sprintf("Question %d of %d", st.at+1, len(qs))
	}
	lines := []string{termrender.Accent("◇ ") + termrender.Bold(title), "  " + oneLine(q.Prompt, m.width-10), ""}
	for i, o := range q.Options {
		mark := "  "
		if slices.Contains(st.picks[st.at], o.Label) {
			mark = termrender.Green("✓ ")
		}
		line := fmt.Sprintf("  %s%s %s", mark, termrender.Accent(termrender.Bold(fmt.Sprint(i+1))), oneLine(o.Label, m.width-14))
		if o.Description != "" {
			line += termrender.Dim(" — " + oneLine(o.Description, max(m.width-16-termrender.VisibleWidth(o.Label), 10)))
		}
		lines = append(lines, line)
	}
	hint := keyHints("1-9", "choose", "enter", "send typed answer", "esc", "decline")
	if q.Multi {
		hint = keyHints("1-9", "toggle", "enter", "confirm", "esc", "decline")
	}
	return card(append(lines, "", hint), m.width, termrender.Accent)
}
