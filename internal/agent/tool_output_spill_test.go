package agent

import (
	"fmt"
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
	for _, lines := range []int{1, 4, 13, 20, 26, 40, 64, 200, 800} {
		body := strings.Repeat(line, lines)
		out, notice := a.boundToolOutput(body, "bash", fmt.Sprintf("call-%d", lines))
		if len(out) > len(body) {
			t.Errorf("%d lines (%d bytes) came back as %d bytes", lines, len(body), len(out))
		}
		if notice == "" && !strings.Contains(out, spillMarker) && out != body {
			t.Errorf("%d lines: neither spilled nor truncated nor returned as-is", lines)
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
	// The whole point is that this stays small. Two instruction blocks plus the
	// rounding of one line is the ceiling; anything near the old 32KB clamp means
	// the cost model regressed into a threshold again.
	if hi > 4096 {
		t.Errorf("highest bar = %d; a pointer costs a few hundred bytes, so the bar belongs near 1KB", hi)
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
		out, _ := a.boundToolOutput(body, "read_file", fmt.Sprintf("earn-%d", lines))
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

func spillBarFor(t *testing.T, lineLen int) int {
	t.Helper()
	a := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ArchiveDir: t.TempDir()}, event.Discard)
	line := strings.Repeat("x", lineLen) + "\n"
	for lines := 1; lines <= 2000; lines++ {
		body := strings.Repeat(line, lines)
		out, _ := a.boundToolOutput(body, "bash", fmt.Sprintf("bar-%d-%d", lineLen, lines))
		if strings.Contains(out, spillMarker) {
			return len(body)
		}
	}
	return 0
}
