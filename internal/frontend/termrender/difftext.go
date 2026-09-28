package termrender

import (
	"strings"

	"reasonix/internal/contract/event"
)

// DiffText renders a whole unified diff — a shell result running `git diff`, say
// — as a header and highlighted body per file section. A configured
// [cli].diff_formatter takes the whole diff on stdin and its stdout is shown
// verbatim, mirroring the fenced-diff and writer-card paths. Rows carry the
// card's body indent; maxLines folds each section.
func DiffText(diff string, width, maxLines int) []string {
	diff = strings.TrimRight(diff, "\n")
	if diff == "" {
		return nil
	}
	if out, ok := renderDiffExternal(activeDiffFormatter, diff+"\n"); ok {
		return indentRows(strings.Split(strings.TrimRight(out, "\n"), "\n"))
	}
	if hasSGR(diff) {
		return verbatimDiffBody(diff, width, maxLines)
	}
	var rows []string
	for _, sec := range splitDiffSections(diff) {
		path := diffFencePath(sec)
		if header := diffFenceHeader(path, countDiff(sec)); header != "" {
			rows = append(rows, "  "+header)
		}
		rows = append(rows, diffBody(event.FileDiff{Diff: sec}, path, width, maxLines)...)
	}
	return rows
}

func indentRows(lines []string) []string {
	rows := make([]string, 0, len(lines))
	for _, ln := range lines {
		rows = append(rows, "  "+ln)
	}
	return rows
}
