package acp

import (
	"strings"
	"testing"
)

// A result the host marked as a whole diff is fenced for the client so it can
// colour +/- lines; an unmarked result is passed through as before.
func TestToolResultTextFencesMarkedDiff(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n"
	got := toolResultText(diff, true)
	if !strings.HasPrefix(got, "```diff\n") || !strings.HasSuffix(got, "\n```") {
		t.Fatalf("marked diff not fenced:\n%q", got)
	}
	if !strings.Contains(got, "+new") {
		t.Fatalf("fence lost the body:\n%q", got)
	}
}

func TestToolResultTextLeavesUnmarkedUnfenced(t *testing.T) {
	if got := toolResultText("plain output\n", false); got != "plain output\n" {
		t.Fatalf("unmarked result changed: %q", got)
	}
}
