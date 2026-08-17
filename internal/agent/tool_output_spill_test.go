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

// The bar for spilling is derived from the pointer that would stay behind, so a
// shape that carries a bigger pointer raises its own bar. Nothing in the package
// is tuned per shape — which is exactly what a constant could not do: change
// spilledOutputHeadLines and every fixed cap becomes wrong silently.
func TestSpillBarFollowsPointerCost(t *testing.T) {
	short := spillBarFor(t, 40)
	long := spillBarFor(t, 120)
	if short == 0 || long == 0 {
		t.Fatal("no bar found; spilling never engaged")
	}
	if long <= short {
		t.Errorf("bar for 120-char lines = %d, for 40-char lines = %d; the longer head must cost more", long, short)
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
