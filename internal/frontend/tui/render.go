package tui

import (
	"fmt"
	"strings"

	"reasonix/internal/contract/event"
	"reasonix/internal/frontend/termrender"
)

const (
	toolPreviewLines = 4
	diffPreviewLines = 24
)

// renderItem is a settled row as it goes into the scrollback. shown is how much
// of an answer's text an earlier print already carried.
func renderItem(it *Item, width, shown int) string {
	switch it.Kind {
	case ItemUser:
		mark := "› "
		if it.Steer {
			mark = "↳ "
		}
		return termrender.Accent(mark) + termrender.Bold(it.Text)
	case ItemSay:
		// Thinking with nothing said after it is a step, not an answer: it gets
		// its marker and no speaker header.
		if strings.TrimSpace(it.Text) == "" {
			if it.Reasoning == "" {
				return ""
			}
			return thoughtLine(it.ThoughtMs)
		}
		if shown >= len(it.Text) && shown > 0 {
			return ""
		}
		out := renderSayPart(it.Text[shown:], shown == 0, width)
		if shown == 0 && it.Reasoning != "" {
			out = thoughtLine(it.ThoughtMs) + "\n" + out
		}
		return out
	case ItemTool:
		return renderTool(it, width)
	case ItemApproval:
		// An allowed call speaks for itself in the card that follows; only a
		// refusal leaves something the reader would otherwise not see.
		if it.Verdict != "deny" && it.Verdict != "revise_plan" && it.Verdict != "exit_plan" {
			return ""
		}
		return termrender.Dim(fmt.Sprintf("  ✗ declined %s %s", it.Approval.Tool, oneLine(it.Approval.Subject, width-20)))
	case ItemAsk:
		prompt := "question"
		if len(it.Ask.Questions) > 0 {
			prompt = it.Ask.Questions[0].Prompt
		}
		return termrender.Dim("  ? " + oneLine(prompt, width/2) + " → " + oneLine(it.Verdict, width/2))
	case ItemNotice:
		return renderNotice(it)
	case ItemCompaction:
		return termrender.Dim("  ⟲ context compacted")
	case ItemReceipt:
		return renderReceipt(it)
	}
	return ""
}

// renderSayPart renders a stretch of an answer. Only the first stretch carries
// the speaker's header; the rest continue under it.
func renderSayPart(text string, first bool, width int) string {
	if first {
		return termrender.AssistantBlock(text, width)
	}
	return indent(strings.TrimRight(termrender.RenderMarkdown(text, max(width-2, 10)), "\n"), "  ")
}

func thoughtLine(ms int64) string {
	if ms <= 0 {
		return termrender.Dim("  ✻ thought")
	}
	return termrender.Dim(fmt.Sprintf("  ✻ thought for %ds", (ms+500)/1000))
}

func renderTool(it *Item, width int) string {
	t := it.Tool
	if t.Diff != "" {
		return strings.Join(termrender.DiffBlock(t.Name, t.Args, event.FileDiff{Diff: t.Diff, Added: t.Added, Removed: t.Removed}, width, diffPreviewLines), "\n")
	}
	lines := []string{termrender.ToolCard(t.Name, t.Args, width)}
	switch {
	case t.Err != "":
		lines = append(lines, "  ⎿ "+termrender.Red(oneLine(t.Err, width-6)))
	case it.Running:
		if last := lastLine(t.Output); last != "" {
			lines = append(lines, termrender.Dim("  ⎿ "+oneLine(last, width-6)))
		}
	default:
		lines = append(lines, previewOutput(t.Output, width)...)
	}
	if n := len(it.Children); n > 0 {
		lines = append(lines, termrender.Dim(fmt.Sprintf("  ⎿ %d sub-agent call(s)", n)))
	}
	return strings.Join(lines, "\n")
}

func previewOutput(out string, width int) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return []string{termrender.Dim("  ⎿ (no output)")}
	}
	src := strings.Split(out, "\n")
	shown := src[:min(len(src), toolPreviewLines)]
	lines := make([]string, 0, len(shown)+1)
	for i, l := range shown {
		gutter := "    "
		if i == 0 {
			gutter = "  ⎿ "
		}
		lines = append(lines, termrender.Dim(gutter+oneLine(l, width-6)))
	}
	if extra := len(src) - len(shown); extra > 0 {
		lines = append(lines, termrender.Dim(fmt.Sprintf("    … +%d lines", extra)))
	}
	return lines
}

func renderNotice(it *Item) string {
	mark := termrender.Dim("  · ")
	switch it.Level {
	case "error":
		mark = termrender.Red("  ✗ ")
	case "warn", "warning":
		mark = termrender.Yellow("  ! ")
	}
	text := it.Text
	if it.Count > 1 {
		text += termrender.Dim(fmt.Sprintf(" (×%d)", it.Count))
	}
	return mark + text
}

func renderReceipt(it *Item) string {
	r := it.Receipt
	parts := []string{r.Verdict}
	if n := len(r.Changes); n > 0 {
		parts = append(parts, fmt.Sprintf("%d change(s)", n))
	}
	if n := len(r.Verifications); n > 0 {
		parts = append(parts, fmt.Sprintf("%d check(s)", n))
	}
	if n := len(r.Gaps); n > 0 {
		parts = append(parts, termrender.Yellow(fmt.Sprintf("%d gap(s)", n)))
	}
	mark := termrender.Green("  ✓ ")
	if r.Verdict != "done" {
		mark = termrender.Yellow("  ! ")
	}
	return mark + strings.Join(parts, termrender.Dim(" · "))
}

func oneLine(s string, width int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if width > 1 && termrender.VisibleWidth(s) > width && len(r) > width-1 {
		return string(r[:width-1]) + "…"
	}
	return s
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func indent(block, prefix string) string {
	lines := strings.Split(block, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}
