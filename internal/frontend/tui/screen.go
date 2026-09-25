package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"reasonix/internal/base/i18n"
	"reasonix/internal/frontend/termrender"
)

const (
	wheelRows    = 3
	flashFor     = 2 * time.Second
	scrollbarCol = 1
)

// screen is the full-screen transcript: settled rows kept here rather than in
// the terminal's scrollback, drawn through a viewport with its own scrollbar,
// wheel and selection. Nil means the rows go to the terminal's scrollback.
type screen struct {
	blocks []block
	yoff   int
	follow bool
	// mouseOff hands the mouse back to the terminal for its own selection.
	mouseOff bool
	sel      selection
	drag     bool
	grab     int
	flash    string
	flashAt  time.Time
}

// block is one settled print, kept as how to draw it so a resize redraws the
// transcript at the new width rather than keeping the old wrapping.
type block struct {
	render func(width int) string
	width  int
	lines  []string
}

func (b *block) at(width int) []string {
	if b.lines == nil || b.width != width {
		b.width, b.lines = width, wrapLines(b.render(width), width)
	}
	return b.lines
}

type selPos struct{ line, col int }

type selection struct {
	active       bool
	anchor, head selPos
}

func (s selection) ordered() (selPos, selPos) {
	a, h := s.anchor, s.head
	if h.line < a.line || (h.line == a.line && h.col < a.col) {
		return h, a
	}
	return a, h
}

func (s selection) empty() bool { return s.anchor == s.head }

type flashDoneMsg struct{}

// wrapLines splits out into rows no wider than width, each padded to it so
// the scrollbar column stays put.
func wrapLines(out string, width int) []string {
	if out == "" {
		return nil
	}
	var rows []string
	for l := range strings.SplitSeq(out, "\n") {
		for r := range strings.SplitSeq(ansi.Hardwrap(l, width, true), "\n") {
			rows = append(rows, termrender.PadRight(r, width))
		}
	}
	return rows
}

// emit sends a settled print where this screen keeps them.
func (m *model) emit(render func(int) string) tea.Cmd {
	if m.scr == nil {
		if out := render(m.width); out != "" {
			return tea.Println(out)
		}
		return nil
	}
	m.scr.blocks = append(m.scr.blocks, block{render: render})
	return nil
}

func (m *model) contentWidth() int { return max(m.width-scrollbarCol, 10) }

// content is every transcript row: the settled blocks, then what is live.
func (m *model) content(live []string) []string {
	cw := m.contentWidth()
	var rows []string
	for i := range m.scr.blocks {
		rows = append(rows, m.scr.blocks[i].at(cw)...)
	}
	return append(rows, wrapLines(strings.Join(live, "\n"), cw)...)
}

// fullView draws the viewport over the transcript with the bottom region
// pinned under it.
func (m *model) fullView(bottom []string, composerAt int) tea.View {
	s := m.scr
	h := max(m.height-len(bottom), 1)
	rows := m.content(m.liveLines())
	total := len(rows)
	if s.follow {
		s.yoff = total - h
	}
	s.yoff = max(min(s.yoff, total-h), 0)
	cw := m.contentWidth()
	blank := strings.Repeat(" ", cw)
	thumbStart, thumbSize := scrollbarThumb(h, s.yoff, total)
	lo, hi := s.sel.ordered()
	out := make([]string, 0, h+len(bottom))
	for r := range h {
		idx := s.yoff + r
		line := blank
		if idx < total {
			line = rows[idx]
		}
		if s.sel.active && !s.sel.empty() {
			if a, b, ok := selSpan(idx, lo, hi, cw); ok {
				line = lipgloss.StyleRanges(line, lipgloss.NewRange(a, b, lipgloss.NewStyle().Reverse(true)))
			}
		}
		out = append(out, line+scrollbarCell(r, total, h, thumbStart, thumbSize))
	}
	for _, l := range bottom {
		out = append(out, ansi.Truncate(l, max(m.width-1, 1), ""))
	}
	v := tea.NewView(strings.Join(out, "\n"))
	v.AltScreen = true
	if !s.mouseOff {
		v.MouseMode = tea.MouseModeCellMotion
	}
	if c := m.composer.Cursor(); c != nil && composerAt >= 0 {
		c.X += 3
		c.Y += h + composerAt + 1
		v.Cursor = c
	}
	return v
}

func (m *model) viewportHeight() int {
	return max(m.height-len(m.bottomLines().rows), 1)
}

func scrollbarThumb(height, yoff, total int) (start, size int) {
	if total <= height {
		return 0, 0
	}
	size = max(height*height/total, 1)
	start = min(yoff*(height-size)/(total-height), height-size)
	return start, size
}

func scrollbarCell(row, total, height, thumbStart, thumbSize int) string {
	if total <= height {
		return " "
	}
	if row >= thumbStart && row < thumbStart+thumbSize {
		return termrender.Accent("█")
	}
	return termrender.Dim("│")
}

// selSpan is the [lo, hi) cell span the selection covers on row idx.
func selSpan(idx int, start, end selPos, cw int) (lo, hi int, ok bool) {
	if idx < start.line || idx > end.line {
		return 0, 0, false
	}
	lo, hi = 0, cw
	if idx == start.line {
		lo = start.col
	}
	if idx == end.line {
		hi = min(end.col, cw)
	}
	return lo, hi, lo < hi
}

// scrollBy moves the viewport and follows the tail again once it reaches it.
func (m *model) scrollBy(n int) {
	s := m.scr
	h := m.viewportHeight()
	total := len(m.content(m.liveLines()))
	s.yoff = max(min(s.yoff+n, total-h), 0)
	s.follow = s.yoff >= total-h
}

// scrollKey takes the keys that move the transcript; they are never text.
func (m *model) scrollKey(k string) bool {
	if m.scr == nil {
		return false
	}
	page := max(m.viewportHeight()-1, 1)
	switch k {
	case "pgup":
		m.scrollBy(-page)
	case "pgdown":
		m.scrollBy(page)
	case "ctrl+home":
		m.scr.yoff, m.scr.follow = 0, false
	case "ctrl+end":
		m.scr.follow = true
	default:
		return false
	}
	return true
}

func (m *model) onMouse(msg tea.MouseMsg) tea.Cmd {
	s := m.scr
	if s == nil || s.mouseOff {
		return nil
	}
	mouse := msg.Mouse()
	h := m.viewportHeight()
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollBy(-wheelRows)
		case tea.MouseWheelDown:
			m.scrollBy(wheelRows)
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseRight && s.sel.active && !s.sel.empty() {
			return m.copySelection()
		}
		if msg.Button != tea.MouseLeft || mouse.Y >= h {
			return nil
		}
		s.sel = selection{}
		if mouse.X >= m.contentWidth() {
			s.drag = true
			s.grab = m.thumbGrab(mouse.Y, h)
			m.dragScrollbar(mouse.Y, h)
			return nil
		}
		at := m.caret(mouse.X, mouse.Y)
		s.sel = selection{active: true, anchor: at, head: at}
	case tea.MouseMotionMsg:
		switch {
		case s.drag:
			m.dragScrollbar(mouse.Y, h)
		case s.sel.active:
			s.sel.head = m.caret(mouse.X, min(max(mouse.Y, 0), h-1))
		}
	case tea.MouseReleaseMsg:
		if s.drag {
			s.drag = false
			return nil
		}
		if s.sel.active {
			if s.sel.empty() {
				s.sel = selection{}
				return nil
			}
			return m.copySelection()
		}
	}
	return nil
}

func (m *model) caret(x, y int) selPos {
	return selPos{line: m.scr.yoff + y, col: min(max(x, 0), m.contentWidth())}
}

func (m *model) thumbGrab(row, h int) int {
	total := len(m.content(m.liveLines()))
	start, size := scrollbarThumb(h, m.scr.yoff, total)
	if row >= start && row < start+size {
		return row - start
	}
	return size / 2
}

func (m *model) dragScrollbar(row, h int) {
	total := len(m.content(m.liveLines()))
	_, size := scrollbarThumb(h, 0, total)
	maxTop := h - size
	if total <= h || maxTop <= 0 {
		return
	}
	top := min(max(row-m.scr.grab, 0), maxTop)
	m.scr.yoff = (top*(total-h) + maxTop/2) / maxTop
	m.scr.follow = m.scr.yoff >= total-h
}

// copySelection puts the selected text on the clipboard; the highlight stays
// as the cue for what was copied.
func (m *model) copySelection() tea.Cmd {
	return termrender.CopyToClipboard(m.selectedText())
}

func (m *model) selectedText() string {
	rows := m.content(m.liveLines())
	lo, hi := m.scr.sel.ordered()
	var picked []string
	for i := lo.line; i <= hi.line && i < len(rows); i++ {
		a, b, ok := selSpan(i, lo, hi, m.contentWidth())
		if !ok {
			continue
		}
		picked = append(picked, strings.TrimRight(ansi.Strip(ansi.Cut(rows[i], a, b)), " "))
	}
	return strings.Join(picked, "\n")
}

// onCopied reports a copy in the footer for a moment.
func (m *model) onCopied(msg termrender.ClipboardCopyMsg) tea.Cmd {
	if msg.Err != nil {
		m.tr.AddNotice("error", "copy: "+msg.Err.Error())
		return m.commit()
	}
	cmds := []tea.Cmd{m.showFlash(i18n.M.MouseCopiedHint)}
	if msg.OSC52 {
		cmds = append(cmds, tea.SetClipboard(msg.Text))
	}
	return tea.Batch(cmds...)
}

func (m *model) showFlash(text string) tea.Cmd {
	if m.scr == nil {
		return nil
	}
	m.scr.flash, m.scr.flashAt = text, time.Now()
	return tea.Tick(flashFor, func(time.Time) tea.Msg { return flashDoneMsg{} })
}

func (m *model) flashText() string {
	if m.scr == nil || m.scr.flash == "" || time.Since(m.scr.flashAt) >= flashFor {
		return ""
	}
	return m.scr.flash
}

// toggleMouse gives the mouse back to the terminal, or takes it again.
func (m *model) toggleMouse() tea.Cmd {
	if m.scr == nil {
		return nil
	}
	s := m.scr
	s.mouseOff, s.sel, s.drag = !s.mouseOff, selection{}, false
	if s.mouseOff {
		return m.showFlash(i18n.M.MouseCaptureOffHint)
	}
	return m.showFlash(i18n.M.MouseCaptureOnHint)
}
