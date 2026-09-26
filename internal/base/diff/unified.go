package diff

import "strings"

// IsUnifiedDiff reports whether text is, in its entirety, a unified diff — the
// shape `git diff` / `diff -u` writes to stdout. Whole-text and conservative:
// the first non-blank line must be a file header and the body must carry a hunk
// and a changed line, so mixed output (`git log -p`, a diff preceded by a log
// line) is rejected and prose is never mistaken for a diff.
func IsUnifiedDiff(text string) bool {
	lines := strings.Split(text, "\n")

	start := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			start = i
			break
		}
	}
	if start < 0 {
		return false
	}

	if first := lines[start]; !strings.HasPrefix(first, "diff --git ") &&
		!strings.HasPrefix(first, "--- ") {
		return false
	}

	hasHunk := false
	hasChange := false
	for _, ln := range lines[start:] {
		switch {
		case strings.HasPrefix(ln, "@@ "):
			hasHunk = true
		case strings.HasPrefix(ln, "+++ "), strings.HasPrefix(ln, "--- "):
			// File headers, not a change line: a removed "-- x" line renders as
			// "--- x" and must not be counted as a deletion.
		case strings.HasPrefix(ln, "+"), strings.HasPrefix(ln, "-"):
			hasChange = true
		}
	}

	return hasHunk && hasChange
}
