package agent

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

const spillMarker = "kept out of context"

// Moving a result out of context must never make the context bigger. A size
// threshold cannot promise this: set one below what the pointer costs and every
// body in between is replaced by something longer than itself.
func TestSpillNeverGrowsTheContext(t *testing.T) {
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: t.TempDir()}, event.Discard)
	line := strings.Repeat("x", 78) + "\n"
	for _, lines := range []int{1, 4, 13, 20, 26, 40, 64, 200, 800, 2000} {
		body := strings.Repeat(line, lines)
		out, notice := a.boundToolOutput(body, "bash", fmt.Sprintf("call-%d", lines), "")
		if len(out) > len(body) {
			t.Errorf("%d lines (%d bytes) came back as %d bytes", lines, len(body), len(out))
		}
		if notice == "" && !strings.Contains(out, spillMarker) && out != body {
			t.Errorf("%d lines: neither spilled nor truncated nor returned as-is", lines)
		}
	}
}

// A result the model could fetch back in a single read has nothing to gain from
// leaving: the pointer and the extra turn buy a trip to where it already was.
// Everyday bash and read_file output lives in this range.
func TestResultsThatFitOneReadBackStayInContext(t *testing.T) {
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: t.TempDir()}, event.Discard)
	for _, size := range []int{1 << 10, 4 << 10, 16 << 10, maxToolOutputBytes} {
		body := strings.Repeat("x", size)
		out, notice := a.boundToolOutput(body, "bash", fmt.Sprintf("fit-%d", size), "")
		if out != body || notice != "" {
			t.Errorf("%d bytes came back changed (notice=%q); one fetch recovers it whole, so it should stay", size, notice)
		}
	}
}

// The pointer's whole promise is that the model can read the result back. A
// fetch that spills returns another pointer to another file, and read_file
// numbers what it returns, so every round is bigger than the last: the content
// never arrives. This is the loop that issued 1936 reads in one session.
func TestReadingASpillBackDoesNotSpillAgain(t *testing.T) {
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: t.TempDir()}, event.Discard)
	body := strings.Repeat(strings.Repeat("payload ", 12)+"\n", 500)
	out, _ := a.boundToolOutput(body, "bash", "call_00_ORIGIN", "")
	if !strings.Contains(out, spillMarker) {
		t.Fatalf("a %d-byte body should have spilled", len(body))
	}
	path := spillPathIn(t, out)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the model cannot read its own spill back: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	args := fmt.Sprintf(`{"path":%q}`, path)

	// A window, the way read_file is meant to be used: it must arrive verbatim.
	window := numberLines(lines[:200])
	got, notice := a.boundToolOutput(window, "read_file", "call_01_WINDOW", args)
	if got != window || notice != "" {
		t.Errorf("a %d-byte window came back changed (notice=%q); the fetch must deliver", len(window), notice)
	}

	// The whole file, repeatedly: bounded however it likes, but never by
	// spilling — that is the step which has no exit.
	for round := 1; round <= 5; round++ {
		whole := numberLines(lines)
		got, _ := a.boundToolOutput(whole, "read_file", fmt.Sprintf("call_%02d_WHOLE", round), args)
		if strings.Contains(got, spillMarker) {
			t.Fatalf("round %d: the fetch was spilled again, so the model gets a pointer where it asked for content", round)
		}
	}
}

// Leaving context is a fixed price, so the bar cannot move with the shape of the
// output. A preview counted in lines would break this: wide lines would make a
// result cost more to move out of context, which is the opposite of the point.
// Bars still round to whole lines, so they are close rather than identical.
func TestSpillBarDoesNotFollowLineLength(t *testing.T) {
	lo, hi := 0, 0
	for _, lineLen := range []int{20, 40, 80, 120, 200} {
		bar := spillBarFor(t, lineLen)
		if bar == 0 {
			t.Fatalf("lineLen=%d: spilling never engaged", lineLen)
		}
		if lo == 0 || bar < lo {
			lo = bar
		}
		if bar > hi {
			hi = bar
		}
		t.Logf("lineLen=%3d bar=%5d", lineLen, bar)
	}
	if hi > 2*lo {
		t.Errorf("bars span %d..%d; a fixed-price pointer must not let line length double the bar", lo, hi)
	}
	// The bar belongs at the point where one fetch stops recovering the whole
	// result. Below it, spilling costs a turn and returns nothing for it.
	if lo <= maxToolOutputBytes {
		t.Errorf("lowest bar = %d, at or under the %d a single fetch carries", lo, maxToolOutputBytes)
	}
	if hi > maxToolOutputBytes+1024 {
		t.Errorf("highest bar = %d; past the cap it should engage within one line's rounding", hi)
	}
}

// At the bar, what the spill saves covers what its pointer costs. Below the bar
// the result stays whole rather than being spilled for a rounding error, since a
// pointer the model reads back costs a further turn.
func TestSpillEarnsAtLeastItsOwnCost(t *testing.T) {
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: t.TempDir()}, event.Discard)
	line := strings.Repeat("y", 96) + "\n"
	for lines := 1; lines <= 600; lines += 3 {
		body := strings.Repeat(line, lines)
		out, _ := a.boundToolOutput(body, "read_file", fmt.Sprintf("earn-%d", lines), "")
		if !strings.Contains(out, spillMarker) {
			continue
		}
		if saved, cost := len(body)-len(out), len(out); saved < cost {
			t.Fatalf("spilled %d bytes to save %d while the pointer costs %d", len(body), saved, cost)
		}
		return
	}
	t.Fatal("spilling never engaged within 600 lines")
}

// Reading something that merely lives elsewhere still spills normally — the
// exemption is about where the path points, not which tool asked.
func TestOrdinaryReadsStillSpill(t *testing.T) {
	root := t.TempDir()
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: root}, event.Discard)
	body := strings.Repeat("a wide line of ordinary file content\n", 2000)
	args := fmt.Sprintf(`{"path":%q}`, root+"/somewhere/else.go")
	out, _ := a.boundToolOutput(body, "read_file", "call-ordinary", args)
	if !strings.Contains(out, spillMarker) {
		t.Errorf("a %d-byte read of an ordinary file should still spill", len(body))
	}
}

func spillBarFor(t *testing.T, lineLen int) int {
	t.Helper()
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: t.TempDir()}, event.Discard)
	line := strings.Repeat("x", lineLen) + "\n"
	for lines := 1; lines <= 4000; lines++ {
		body := strings.Repeat(line, lines)
		out, _ := a.boundToolOutput(body, "bash", fmt.Sprintf("bar-%d-%d", lineLen, lines), "")
		if strings.Contains(out, spillMarker) {
			return len(body)
		}
	}
	return 0
}

func spillPathIn(t *testing.T, pointer string) string {
	t.Helper()
	for _, l := range strings.Split(pointer, "\n") {
		if rest, ok := strings.CutPrefix(l, "Full output: "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("no path in pointer:\n%s", pointer)
	return ""
}

// numberLines mimics read_file's output shape, whose per-line prefixes are why
// an unchecked read-back grows instead of converging.
func numberLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%4d→%s\n", i+1, l)
	}
	return b.String()
}
