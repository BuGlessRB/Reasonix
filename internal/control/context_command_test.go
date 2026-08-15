package control

import (
	"strings"
	"testing"

	"reasonix/internal/agent"
)

// "70% full" does not tell the user whether that is close to anything, so the
// summary names the next action for the pressure it reports.
func TestContextSummaryNamesTheNextAction(t *testing.T) {
	base := agent.ContextReport{Window: 1000, FoldThreshold: 800, HardCeiling: 900}
	for _, tc := range []struct {
		name   string
		prompt int
		want   string
	}{
		// An empty session used to report the force threshold: the retired
		// levels were never populated, so "prompt >= 0" matched first.
		{"empty session", 0, "fold at 80%"},
		{"below the fold trigger", 100, "fold at 80%"},
		{"at the fold trigger", 800, "folding"},
		{"at the hard ceiling", 950, "at the hard ceiling"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := base
			rep.LatestPrompt = tc.prompt
			if got := contextSummaryLine(rep); !strings.Contains(got, tc.want) {
				t.Errorf("summary = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

// A blocked maintenance pass is the thing users cannot currently see, so it has
// to reach the one-line summary rather than only the detail block.
func TestContextSummaryFlagsBlockedMaintenance(t *testing.T) {
	rep := agent.ContextReport{
		Window: 1000, FoldThreshold: 800, HardCeiling: 900,
		LatestPrompt: 700, BlockedReason: "context summary failed: deadline exceeded",
	}
	if got := contextSummaryLine(rep); !strings.Contains(got, "blocked") {
		t.Errorf("summary = %q, want it to flag the blocked pass", got)
	}
}

func TestThousandsGroupsDigits(t *testing.T) {
	for in, want := range map[int]string{
		0: "0", 999: "999", 1000: "1,000", 731281: "731,281", 1048576: "1,048,576", -1234: "-1,234",
	} {
		if got := thousands(in); got != want {
			t.Errorf("thousands(%d) = %q, want %q", in, got, want)
		}
	}
}
