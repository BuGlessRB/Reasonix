package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/contract/eventwire"
)

// newViModel builds an idle TUI whose composer uses the vi command mode.
func newViModel(t *testing.T) (*model, *recordingKernel) {
	t.Helper()
	m, k := testModel(t)
	m.opts.CommandMode = true
	return m, k
}

func viKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

var (
	viCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	viCtrlD = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
)

func TestViEscEntersCommandModeAndInsertLeavesIt(t *testing.T) {
	m, _ := newViModel(t)
	if m.viInCommand() {
		t.Fatal("should start in insert mode")
	}
	press(m, "esc")
	if !m.viInCommand() {
		t.Fatal("esc should enter command mode")
	}
	// A command-mode key (i) returns to insert editing.
	m.Update(viKey('i'))
	if m.viInCommand() {
		t.Fatal("i should leave command mode")
	}
}

func TestViEscLeavingInsertStepsCaretLeft(t *testing.T) {
	m, _ := newViModel(t)
	m.composer.SetValue("abc")
	m.composer.SetCursorColumn(3) // after the last char, as after typing "abc"
	press(m, "esc")
	if !m.viInCommand() {
		t.Fatal("esc should enter command mode")
	}
	if got := m.composer.Column(); got != 2 {
		t.Fatalf("caret after esc from insert = %d, want 2 (one rune left of end)", got)
	}
	// A second Esc while already in command mode must not move the caret.
	press(m, "esc")
	if !m.viInCommand() {
		t.Fatal("esc must keep command mode")
	}
	if got := m.composer.Column(); got != 2 {
		t.Fatalf("caret after second esc = %d, want 2 (no-op)", got)
	}
}

func TestViDeleteCharX(t *testing.T) {
	m, _ := newViModel(t)
	m.composer.SetValue("abc")
	m.composer.SetCursorColumn(0)
	press(m, "esc")
	if !m.viInCommand() {
		t.Fatal("esc should enter command mode")
	}
	m.Update(viKey('x'))
	if got := m.composer.Value(); got != "bc" {
		t.Fatalf("x at col 0 = %q, want bc", got)
	}
	if !m.viInCommand() {
		t.Fatal("x must stay in command mode")
	}
	// Pressing an unrecognized command-mode key must not insert text.
	m.Update(viKey('q'))
	if got := m.composer.Value(); got != "bc" {
		t.Fatalf("command mode ignored 'q' but value = %q, want bc", got)
	}
	if !m.viInCommand() {
		t.Fatal("must remain in command mode after ignored key")
	}
}

func TestViDeleteCharAtEndOfLineNoop(t *testing.T) {
	m, _ := newViModel(t)
	m.composer.SetValue("abc")
	m.composer.SetCursorColumn(3) // caret past last char
	press(m, "esc")
	// Esc from insert steps left; move back to the true end in command mode.
	m.Update(viKey('$'))
	if got := m.composer.Column(); got != 3 {
		t.Fatalf("$ caret col = %d, want 3", got)
	}
	m.Update(viKey('x'))
	if got := m.composer.Value(); got != "abc" {
		t.Fatalf("x at end-of-line = %q, want abc unchanged", got)
	}
}

func TestViInsertAfterLastChar(t *testing.T) {
	m, _ := newViModel(t)
	m.composer.SetValue("abc")
	m.composer.SetCursorColumn(1) // caret after 'a'
	press(m, "esc")
	m.Update(viKey('A')) // append after last char
	if m.viInCommand() {
		t.Fatal("A should enter insert mode")
	}
	if got := m.composer.Column(); got != 3 {
		t.Fatalf("A caret col = %d, want 3 (end of 'abc')", got)
	}
	m.Update(viKey('d'))
	if got := m.composer.Value(); got != "abcd" {
		t.Fatalf("typing after A = %q, want abcd", got)
	}
}

func TestViInsertBeforeFirstChar(t *testing.T) {
	m, _ := newViModel(t)
	m.composer.SetValue("bc")
	m.composer.SetCursorColumn(2) // caret at end
	press(m, "esc")
	m.Update(viKey('I')) // insert before first char
	if m.viInCommand() {
		t.Fatal("I should enter insert mode")
	}
	m.Update(viKey('a'))
	if got := m.composer.Value(); got != "abc" {
		t.Fatalf("typing after I = %q, want abc", got)
	}
}

func TestViCtrlDQuitOnlyInInsertModeEmptyPrompt(t *testing.T) {
	// Insert mode, truly empty prompt: quits.
	m, _ := newViModel(t)
	_, cmd := m.Update(viCtrlD)
	if cmd == nil {
		t.Fatal("^D in insert mode on empty prompt should request shutdown")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Fatal("^D on an empty prompt did not request shutdown")
	}

	// Insert mode but with content (even a single space): no quit.
	m, _ = newViModel(t)
	m.composer.SetValue(" ")
	if _, cmd := m.Update(viCtrlD); cmd != nil {
		t.Fatal("^D with a space must not quit in vi mode")
	}

	// Command mode on the empty prompt: no quit.
	m, _ = newViModel(t)
	press(m, "esc")
	if _, cmd := m.Update(viCtrlD); cmd != nil {
		t.Fatal("^D in command mode must not quit")
	}
}

func TestViCtrlCLeavesCommandModeButNeverQuits(t *testing.T) {
	m, _ := newViModel(t)
	press(m, "esc")
	if !m.viInCommand() {
		t.Fatal("setup: expected command mode")
	}
	_, cmd := m.Update(viCtrlC)
	if m.viInCommand() {
		t.Fatal("^C should leave command mode back into insert")
	}
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("^C on idle must not quit")
		}
	}
}

// TestViCtrlCSavesDraftToHistoryAndClears: pressing ^C at the prompt with typed
// text must save the draft to the cmdline history verbatim and clear the prompt,
// so an accidental interrupt never loses it.
func TestViCtrlCSavesDraftToHistoryAndClears(t *testing.T) {
	m, _ := newViModel(t)
	m.composer.SetValue("half-written draft")

	m.Update(viCtrlC)

	if got := m.composer.Value(); got != "" {
		t.Fatalf("prompt after ^C = %q, want empty", got)
	}
	if len(m.history) == 0 || m.history[len(m.history)-1] != "half-written draft" {
		t.Fatalf("draft not saved to cmdline history: %v", m.history)
	}
}

func TestViAskIgnoresEsc(t *testing.T) {
	// vi mode: Esc must not dismiss the ask card.
	m, _ := newViModel(t)
	apply(m, askEvent())
	if m.tr.OpenPrompt() == nil {
		t.Fatal("setup: expected an open ask")
	}
	press(m, "esc")
	if m.tr.OpenPrompt() == nil {
		t.Fatal("vi mode: Esc must not dismiss the ask card")
	}

	// vi mode: Esc while typing an answer must not back out / discard.
	m2, _ := newViModel(t)
	apply(m2, askEvent())
	m2.openAsk(m2.tr.OpenPrompt())
	m2.ask.entry = entryAnswer
	m2.composer.SetValue("draft")
	press(m2, "esc")
	if m2.ask == nil || !m2.ask.entering() {
		t.Fatal("vi mode: Esc while typing an ask answer must not back out")
	}
	if got := m2.composer.Value(); got != "draft" {
		t.Fatalf("vi mode: Esc discarded the draft, value = %q, want draft", got)
	}
}

func TestAskEscStillDismissesWhenNotVi(t *testing.T) {
	m, _ := testModel(t) // commandmode empty → not vi
	if m.viActive() {
		t.Fatal("setup: expected non-vi mode")
	}
	apply(m, askEvent())
	run(m, press(m, "esc"))
	if m.tr.OpenPrompt() != nil {
		t.Fatal("non-vi: Esc must still dismiss the ask card")
	}
}

// TestViEscDoesNotCancelRunningTurn keeps vi mode's Esc a pure mode switch:
// while a turn runs, Esc is a no-op and never cancels — only ^C interrupts.
func TestViEscDoesNotCancelRunningTurn(t *testing.T) {
	m, k := newViModel(t)
	apply(m, eventwire.Event{Kind: "turn_started"})
	if !m.tr.Running {
		t.Fatal("setup: expected a running turn")
	}
	press(m, "esc")
	if !m.tr.Running {
		t.Fatal("vi mode: Esc must not cancel a running turn")
	}
	if m.viInCommand() {
		t.Fatal("vi mode: Esc while a turn runs must not enter command mode")
	}
	if calls := strings.Join(k.seen(), "\n"); strings.Contains(calls, "POST /cancel") {
		t.Fatal("vi mode: Esc cancelled the turn")
	}
}
