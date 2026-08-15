package cli

import (
	"strings"
	"testing"

	"reasonix/internal/event"
)

// TestCompactionCardLines locks the finished-compaction card: a header naming
// the count and trigger, the summary under a gutter, and an archive line.
func TestCompactionCardLines(t *testing.T) {
	lines := compactionCardLines(event.Compaction{
		Trigger:  "auto",
		Messages: 12,
		Summary:  "## Goal\n- do X\n## Files & code\n- a.go edited",
		Archive:  "/tmp/arch/20260531.jsonl",
	})

	joined := strings.Join(lines, "\n")
	if !strings.Contains(lines[0], "◆") {
		t.Errorf("header should carry the card glyph, got %q", lines[0])
	}
	for _, want := range []string{"Context compacted", "12", "auto"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header missing %q: %q", want, lines[0])
		}
	}
	// Every summary line (and the archive line) sits under the "│" gutter.
	for _, want := range []string{"│ ## Goal", "│ - do X", "│ - a.go edited", "│ archived /tmp/arch/20260531.jsonl"} {
		if !strings.Contains(joined, want) {
			t.Errorf("card missing gutter line %q in:\n%s", want, joined)
		}
	}
}

// A digest reads as complete whatever it dropped, so the count of the fold's
// changes it actually carried is the one thing the card can say that the
// summary text cannot.
func TestCompactionCardShowsWhatTheDigestKept(t *testing.T) {
	joined := strings.Join(compactionCardLines(event.Compaction{
		Trigger: "auto", Messages: 12, Summary: "- brief",
		SourceTokens: 128_000, ProjectionTokens: 31_200,
		CoverageRequired: 12, CoverageMissing: 2, CoverageRepaired: true,
	}), "\n")

	for _, want := range []string{"128.0K", "31.2K", "10/12", "repaired"} {
		if !strings.Contains(joined, want) {
			t.Errorf("quality line missing %q in:\n%s", want, joined)
		}
	}
}

// A fold that produced no changes has no coverage to report, and a card that
// prints "0/0 changes kept" reads as a loss where there was nothing to lose.
func TestCompactionCardOmitsQualityWithoutCoverage(t *testing.T) {
	joined := strings.Join(compactionCardLines(event.Compaction{
		Trigger: "manual", Messages: 3, Summary: "- brief",
	}), "\n")
	if strings.Contains(joined, "changes kept") || strings.Contains(joined, "→") {
		t.Errorf("card invented a quality line with nothing to report:\n%s", joined)
	}
}

// TestCompactionCardLinesNoArchive omits the archive line when none was written.
func TestCompactionCardLinesNoArchive(t *testing.T) {
	lines := compactionCardLines(event.Compaction{Trigger: "manual", Messages: 3, Summary: "- brief"})
	if strings.Contains(strings.Join(lines, "\n"), "archived") {
		t.Errorf("no archive path should mean no archive line: %v", lines)
	}
	if !strings.Contains(lines[0], "manual") {
		t.Errorf("header should reflect the manual trigger: %q", lines[0])
	}
}

// The live line shows the newest line of the digest, not all of it: a digest is
// thousands of tokens, and a status line that grows with it stops being one.
func TestCompactionProgressLineKeepsOnlyTheNewestLine(t *testing.T) {
	if got := lastCompactionLine("## Goal\n- ship the parser\n## Files"); got != "## Files" {
		t.Fatalf("tail = %q, want the newest line", got)
	}
	if got := lastCompactionLine("## Goal\n- ship the parser\n"); got != "- ship the parser" {
		t.Fatalf("a trailing newline must not blank the line: %q", got)
	}
	long := strings.Repeat("每一处改动都要写进简报", 40)
	if got := []rune(lastCompactionLine(long)); len(got) > 120 {
		t.Fatalf("tail is %d runes, want it bounded", len(got))
	}
}
