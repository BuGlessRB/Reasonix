package tui

import (
	"strings"
	"testing"
)

// The picker opens on the first conversation that is not the open one, what
// is typed narrows the list, and enter resumes the row under the cursor and
// reads it back.
func TestPickerResumesTheChosenSession(t *testing.T) {
	m, k := testModel(t)
	run(m, m.openPicker())
	if m.picker == nil || m.picker.sel != 1 {
		t.Fatalf("picker = %+v", m.picker)
	}
	v := m.View().Content
	if !strings.Contains(v, "(active)") || strings.Contains(v, "❯ ") && !strings.Contains(v, "❯ 2.") {
		t.Fatalf("picker should start on the second row:\n%s", v)
	}
	typeText(m, "fix")
	if got := m.picker.shown(); len(got) != 2 {
		t.Fatalf("filter kept %d rows", len(got))
	}
	run(m, press(m, "down"))
	run(m, press(m, "enter"))
	calls := strings.Join(k.seen(), "\n")
	if !strings.Contains(calls, `POST /resume {"path":"/s/c.jsonl"}`) || !strings.Contains(calls, "GET /history") {
		t.Fatalf("resume did not bind and reload:\n%s", calls)
	}
	if m.picker != nil || len(m.tr.Items) == 0 || m.tr.Items[0].Text != "write the docs" {
		t.Fatalf("transcript after resume = %+v", m.tr.Items)
	}
}

func TestPickerIgnoresTheOpenSession(t *testing.T) {
	m, k := testModel(t)
	run(m, m.openPicker())
	m.picker.sel = 0
	run(m, press(m, "enter"))
	if strings.Contains(strings.Join(k.seen(), "\n"), "POST /resume") {
		t.Fatal("picking the open session resumed it again")
	}
}

// /resume and /mouse are answered here, so the menu offers them beside the
// kernel's commands and the composer never sends them.
func TestLocalCommandsAreOfferedAndKeptLocal(t *testing.T) {
	m, k := testModel(t)
	m.composer.SetValue("/re")
	m.onCompletion(completionMsg{line: "/re"})
	if m.menu == nil || m.menu.c.Items[0].Label != "/resume" {
		t.Fatalf("menu = %+v", m.menu)
	}
	run(m, press(m, "enter"))
	if got := m.composer.Value(); got != "/resume" {
		t.Fatalf("composer = %q", got)
	}
	run(m, press(m, "enter"))
	if m.picker == nil || strings.Contains(strings.Join(k.seen(), "\n"), "POST /submit") {
		t.Fatal("/resume went to the kernel instead of opening the picker")
	}
}
