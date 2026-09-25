package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/contract/eventwire"
)

func fillTranscript(m *model, n int) {
	for i := range n {
		m.tr.AddNotice("info", fmt.Sprintf("row %02d", i))
	}
	m.commit()
}

// The transcript follows new output until the user scrolls away, and picks
// the tail up again once they scroll back down to it.
func TestFullScreenScrollsAndFollowsTheTail(t *testing.T) {
	m, _ := testModel(t)
	fillTranscript(m, 60)
	if v := m.View(); !v.AltScreen || !strings.Contains(v.Content, "row 59") || !strings.Contains(v.Content, "█") {
		t.Fatalf("full screen should show the tail with a scrollbar:\n%s", v.Content)
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m.tr.AddNotice("info", "row new")
	m.commit()
	if v := m.View().Content; strings.Contains(v, "row new") || m.scr.follow {
		t.Fatalf("a scrolled-back view jumped to new output:\n%s", v)
	}
	press(m, "ctrl+end")
	if v := m.View().Content; !strings.Contains(v, "row new") {
		t.Fatalf("ctrl+end did not return to the tail:\n%s", v)
	}
	press(m, "ctrl+home")
	if v := m.View().Content; !strings.Contains(v, "reasonix") && !strings.Contains(v, "row 00") {
		t.Fatalf("ctrl+home did not reach the top:\n%s", v)
	}
}

// Dragging the thumb to the bottom of the track lands on the last page.
func TestScrollbarDragMovesTheView(t *testing.T) {
	m, _ := testModel(t)
	fillTranscript(m, 60)
	press(m, "ctrl+home")
	m.View()
	x := m.contentWidth()
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: 0})
	m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x, Y: 40})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: 40})
	if !m.scr.follow {
		t.Fatalf("thumb dragged to the end left the view at %d", m.scr.yoff)
	}
}

// A drag selects transcript text and its release copies it, without the
// padding the scrollbar column needs.
func TestDragSelectsAndCopiesTranscriptText(t *testing.T) {
	m, _ := testModel(t)
	apply(m, eventwire.Event{Kind: "notice", Level: "info", Text: "alpha beta"})
	m.View()
	y := strings.Index(strings.Join(m.content(nil), "\n"), "alpha")
	row := strings.Count(strings.Join(m.content(nil), "\n")[:y], "\n") - m.scr.yoff
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: row})
	m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 30, Y: row})
	if got := m.selectedText(); !strings.Contains(got, "alpha beta") || strings.HasSuffix(got, " ") {
		t.Fatalf("selected %q", got)
	}
	_, cmd := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 30, Y: row})
	if cmd == nil {
		t.Fatal("release did not copy")
	}
}

// Settled rows keep how to draw them, so a narrower window rewraps them.
func TestResizeRewrapsSettledRows(t *testing.T) {
	m, _ := testModel(t)
	m.tr.AddNotice("info", strings.Repeat("word ", 30))
	m.commit()
	wide := len(m.content(nil))
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	if narrow := len(m.content(nil)); narrow <= wide {
		t.Fatalf("rows at 40 cols = %d, at 80 = %d", narrow, wide)
	}
}

func TestInlineWritesToTheTerminalScrollback(t *testing.T) {
	m, _ := testModel(t)
	m.scr = nil
	m.tr.AddNotice("info", "printed")
	if cmd := m.commit(); cmd == nil {
		t.Fatal("inline commit printed nothing")
	}
	if m.View().AltScreen {
		t.Fatal("inline mode took the full screen")
	}
}
