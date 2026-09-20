package cli

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"reasonix/internal/event"
)

func TestSplitDiffFormatter(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"delta --color-only --paging=never", []string{"delta", "--color-only", "--paging=never"}},
		{`delta --syntax-theme "Monokai Extended"`, []string{"delta", "--syntax-theme", "Monokai Extended"}},
		{`my tool 'a b' c`, []string{"my", "tool", "a b", "c"}},
		{`x ""`, []string{"x", ""}},
	}
	for _, tc := range cases {
		if got := splitDiffFormatter(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitDiffFormatter(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func TestRenderDiffExternalPassesStdinToStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no portable stdin→stdout filter")
	}
	out, ok := renderDiffExternal([]string{"cat"}, "--- a/x\n+++ b/x\n")
	if !ok {
		t.Fatal("renderDiffExternal(cat) failed")
	}
	if out != "--- a/x\n+++ b/x\n" {
		t.Fatalf("stdout = %q", out)
	}
}

func TestRenderDiffExternalFallsBackOnError(t *testing.T) {
	if _, ok := renderDiffExternal([]string{"reasonix-no-such-command-xyz"}, "diff"); ok {
		t.Fatal("expected ok=false for a missing command")
	}
	if _, ok := renderDiffExternal(nil, "diff"); ok {
		t.Fatal("expected ok=false for empty argv")
	}
	if _, ok := renderDiffExternal([]string{"cat"}, ""); ok {
		t.Fatal("expected ok=false for empty diff")
	}
	if _, ok := renderDiffExternal([]string{"cat"}, strings.Repeat("x", diffFormatMaxBytes+1)); ok {
		t.Fatal("expected ok=false for an oversized diff")
	}
}

func TestDiffBodyVerbatimWhenColourised(t *testing.T) {
	// SGR-coloured input is shown as-is (no line-number gutter added).
	d := event.FileDiff{Diff: "\x1b[32m+added\x1b[0m\n\x1b[31m-removed\x1b[0m\n"}
	rows := diffBody(d, "x.go", 80, 0)
	if len(rows) != 2 {
		t.Fatalf("want 2 verbatim rows, got %d: %#v", len(rows), rows)
	}
	for _, r := range rows {
		if !strings.Contains(r, "\x1b[") {
			t.Fatalf("verbatim row lost its colour: %q", r)
		}
		if strings.Contains(r, "\x1b[48;5;") {
			t.Fatalf("verbatim row should not carry a background bar: %q", r)
		}
	}
}

func TestSGROnlyKeepsColourDropsHijack(t *testing.T) {
	in := "\x1b[32mgreen\x1b[0m\x1b]52;c;aGk=\x07\x1b[2Jtext"
	got := sgrOnly(in)
	if !strings.Contains(got, "\x1b[32m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("SGR dropped: %q", got)
	}
	if strings.Contains(got, "\x1b]") || strings.Contains(got, "\x1b[2J") {
		t.Fatalf("non-SGR control sequence leaked: %q", got)
	}
	if !strings.Contains(got, "green") || !strings.Contains(got, "text") {
		t.Fatalf("visible text lost: %q", got)
	}
}

func TestRenderDiffFenceVerbatimWhenColourised(t *testing.T) {
	defer func(prev colorprofile.Profile) { activeColorProfile = prev }(activeColorProfile)
	activeColorProfile = colorprofile.ANSI256
	r := newMarkdownRenderer(80)
	out := r.Render("```diff\n\x1b[32m+added\x1b[0m\n\x1b[31m-removed\x1b[0m\n```\n")
	if !strings.Contains(out, "\x1b[32m+added") || !strings.Contains(out, "\x1b[31m-removed") {
		t.Fatalf("coloured diff fence lost its SGR:\n%q", out)
	}
	if strings.Contains(out, "\x1b[48;5;") {
		t.Fatalf("coloured diff fence should not be re-parsed into background bars:\n%q", out)
	}
}

// A configured [cli].diff_formatter runs the whole fence body through the
// command and its stdout is re-emitted untouched — no gutter, no width clamp,
// and no escape filtering, so every sequence the formatter emitted survives.
func TestRenderDiffFenceRunsConfiguredFormatterVerbatim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no portable stdin→stdout filter")
	}
	defer func(prev []string) { activeDiffFormatter = prev }(activeDiffFormatter)
	activeDiffFormatter = []string{"cat"}

	r := newMarkdownRenderer(80)
	body := "--- a/x\n+++ b/x\n\x1b]52;c;aGk=\x07\x1b[31m-red\x1b[0m\n"
	out := r.Render("```diff\n" + body + "```\n")
	if !strings.Contains(out, "\x1b]52;c;aGk=\x07") {
		t.Fatalf("formatter output was filtered:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[31m-red\x1b[0m") {
		t.Fatalf("formatter output lost its SGR:\n%q", out)
	}
	if strings.Contains(out, "\x1b[48;5;") {
		t.Fatalf("formatted fence should not be re-rendered with background bars:\n%q", out)
	}
}

// Without a configured formatter the fence keeps the built-in renderer.
func TestRenderDiffFenceFallsBackWithoutFormatter(t *testing.T) {
	defer func(prev []string) { activeDiffFormatter = prev }(activeDiffFormatter)
	activeDiffFormatter = nil

	r := newMarkdownRenderer(80)
	out := r.Render("```diff\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n```\n")
	if !strings.Contains(out, "new") || !strings.Contains(out, "old") {
		t.Fatalf("built-in renderer lost the diff body:\n%q", out)
	}
}

// A configured [cli].diff_formatter also formats a writer tool's diff card: the
// whole diff is piped through the command and its stdout is re-emitted verbatim
// under the card header — no gutter, no width clamp, no escape filtering, and
// no fold (maxLines is ignored), mirroring the fenced path.
func TestDiffBlockRunsConfiguredFormatterVerbatim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no portable stdin→stdout filter")
	}
	defer func(prev []string) { activeDiffFormatter = prev }(activeDiffFormatter)
	activeDiffFormatter = []string{"cat"}

	d := event.FileDiff{
		Diff:    "--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n\x1b]52;c;aGk=\x07\x1b[31m-old\x1b[0m\n+new\n",
		Added:   1,
		Removed: 1,
	}
	block := diffBlock("edit_file", `{"path":"pkg/x.go"}`, d, 80, 1)
	if len(block) == 0 || !strings.Contains(block[0], "pkg/x.go") {
		t.Fatalf("header should name the path, got %q", block[0])
	}
	body := strings.Join(block[1:], "\n")
	if !strings.Contains(body, "\x1b]52;c;aGk=\x07") {
		t.Fatalf("formatter output was filtered:\n%q", body)
	}
	if !strings.Contains(body, "--- a/x.go") || !strings.Contains(body, "\x1b[31m-old\x1b[0m") {
		t.Fatalf("formatter output should keep the raw diff, got:\n%q", body)
	}
	if strings.Contains(body, "\x1b[48;5;") {
		t.Fatalf("formatted card should not be re-rendered with background bars:\n%q", body)
	}
	if strings.Contains(body, "more lines") {
		t.Fatalf("formatted card should not fold even past maxLines:\n%q", body)
	}
	for _, row := range block[1:] {
		if !strings.HasPrefix(row, "  ") {
			t.Fatalf("body row should be indented to the card column, got %q", row)
		}
	}
}

// Without a configured formatter the writer diff card keeps the built-in
// renderer (headers dropped, line-number gutter added).
func TestDiffBlockFallsBackWithoutFormatter(t *testing.T) {
	defer func(prev []string) { activeDiffFormatter = prev }(activeDiffFormatter)
	activeDiffFormatter = nil

	d := event.FileDiff{Diff: "--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-old\n+new\n", Added: 1, Removed: 1}
	body := strings.Join(diffBlock("edit_file", `{"path":"x.go"}`, d, 80, 40)[1:], "\n")
	if !strings.Contains(body, "old") || !strings.Contains(body, "new") {
		t.Fatalf("built-in renderer lost the diff body:\n%q", body)
	}
	if strings.Contains(body, "--- a/x.go") || strings.Contains(body, "+++ b/x.go") {
		t.Fatalf("built-in renderer should drop the file headers:\n%q", body)
	}
}

// A formatter that fails to run leaves the writer diff card on the built-in
// renderer.
func TestDiffBlockFallsBackWhenFormatterFails(t *testing.T) {
	defer func(prev []string) { activeDiffFormatter = prev }(activeDiffFormatter)
	activeDiffFormatter = []string{"reasonix-no-such-command-xyz"}

	d := event.FileDiff{Diff: "--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-old\n+new\n", Added: 1, Removed: 1}
	body := strings.Join(diffBlock("edit_file", `{"path":"x.go"}`, d, 80, 40)[1:], "\n")
	if !strings.Contains(body, "old") || !strings.Contains(body, "new") {
		t.Fatalf("built-in renderer lost the diff body:\n%q", body)
	}
}
