package cli

import (
	"encoding/base64"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

const ()

const assistantTranscriptIndent = "  "

// renderAssistantMarkdown gives assistant prose the same explicit transcript
// identity that user, reasoning, tool, and receipt blocks already have. The
// body keeps a restrained two-cell gutter instead of using a heavy card, and
// rendering at the reduced width keeps every indented row inside the viewport.
func renderAssistantMarkdown(raw string, contentWidth int) string {
	contentWidth = max(contentWidth, 1)
	indent := assistantTranscriptIndent
	if contentWidth <= visibleWidth(indent) {
		indent = ""
	}
	bodyWidth := max(contentWidth-visibleWidth(indent), 1)
	renderer := newMarkdownRenderer(bodyWidth)
	rendered := renderer.Render(raw)
	if rendered == "" {
		rendered = raw
	}
	body := strings.TrimRight(rendered, "\n")
	header := indent + accent("◆") + " " + bold("Reasonix")
	if body == "" {
		return header
	}
	return header + "\n\n" + indentTranscriptBlock(body, indent)
}

func indentTranscriptBlock(block, indent string) string {
	if indent == "" || block == "" {
		return block
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

const (
	copyMathStartPrefix = "\x1b]1337;reasonix-copy-math="
	copyMathEndPrefix   = "\x1b]1337;reasonix-copy-math-end="
	copyMathTerminator  = "\x07"
)

func copyMathStartMarker(id, source string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(source))
	return copyMathStartPrefix + id + ";" + encoded + copyMathTerminator
}

func copyMathEndMarker(id string) string {
	return copyMathEndPrefix + id + copyMathTerminator
}

type clipboardCopyMsg struct {
	text       string
	err        error
	osc52      bool
	statusHint bool
	seq        int
}

var writeNativeClipboardText = clipboard.WriteAll

func remoteClipboardSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != ""
}

// copyToClipboard prefers the operating system clipboard in a local session,
// where success can be verified (pbcopy on macOS, the selected Wayland/X11
// utility on Linux, and the Win32 clipboard on Windows). SSH cannot reliably
// reach the user's local desktop clipboard, so it deliberately falls back to
// OSC 52. A failed local write also falls back, but the UI labels that path as
// an unverified terminal request rather than claiming a successful copy.
func copyToClipboard(text string) tea.Cmd {
	return copyToClipboardWithStatus(text, 0, false)
}

func copyToClipboardWithStatus(text string, seq int, statusHint bool) tea.Cmd {
	return func() tea.Msg {
		if remoteClipboardSession() {
			return clipboardCopyMsg{text: text, osc52: true, statusHint: statusHint, seq: seq}
		}
		return clipboardCopyMsg{
			text:       text,
			err:        writeNativeClipboardText(text),
			statusHint: statusHint,
			seq:        seq,
		}
	}
}

// edgeScrollDir reports the auto-scroll direction for a drag at screen row y in
// a viewport of `height` rows: -1 at the top edge, +1 at the bottom, 0 between.
func edgeScrollDir(y, height int) int {
	switch {
	case y <= 0:
		return -1
	case y >= height-1:
		return 1
	default:
		return 0
	}
}

// selPos is a caret position in the wrapped transcript: a content-line index
// (absolute, scroll-independent) and a visual column.
type selPos struct{ line, col int }

var ()

// selSpan returns the [lo, hi) visual-column span of the selection on content
// line idx (false when the line is outside the selection). cw bounds the span
// so a multi-line selection highlights through the right edge.
func selSpan(idx int, start, end selPos, cw int) (lo, hi int, ok bool) {
	if idx < start.line || idx > end.line {
		return 0, 0, false
	}
	lo, hi = 0, cw
	if idx == start.line {
		lo = start.col
	}
	if idx == end.line {
		hi = end.col
	}
	if hi > cw {
		hi = cw
	}
	if lo >= hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// scrollbarThumb returns the thumb's [start, start+size) row span for a viewport
// of `height` rows showing `total` content lines scrolled to `yoff`.
func scrollbarThumb(height, yoff, total int) (start, size int) {
	if total <= height {
		return 0, 0 // no overflow → no thumb
	}
	size = max(height*height/total, 1)
	maxYoff := total - height
	start = min(yoff*(height-size)/maxYoff, height-size)
	return start, size
}
