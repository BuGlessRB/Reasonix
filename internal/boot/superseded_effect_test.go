package boot

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// isSummarizerRequest tells the compaction summarizer's own call apart from the
// agent's. Only the agent's requests carry the projection under test.
func isSummarizerRequest(req provider.Request) bool {
	return len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "compacting the earlier part")
}

func countUserTag(req provider.Request, tag string) int {
	n := 0
	for _, m := range req.Messages {
		if m.Role == provider.RoleUser {
			n += strings.Count(m.Content, tag)
		}
	}
	return n
}

// TestEffectFoldDropsSupersededStandingState pins the invariant at the boundary
// that decides it: the host restates standing state on every user turn, and a
// fold must drop the copies it superseded. Without that the count only ever
// grows - retained user turns carry their copy through every later fold - so a
// long session pays for the same block once per turn it ever ran.
func TestEffectFoldDropsSupersededStandingState(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &compactionEffectProvider{bulk: strings.Repeat("work output line with detail. ", 400)}
	provider.Register("boot-superseded-effect", func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
compact_ratio = 0.5
recent_keep = 2

[[providers]]
name = "test-model"
kind = "boot-superseded-effect"
model = "x"
context_window = 32000
`)

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	for _, prompt := range []string{"start", "keep going", "keep going", "keep going",
		"keep going", "keep going", "keep going", "keep going"} {
		if err := ctrl.Run(context.Background(), prompt); err != nil {
			t.Fatalf("Run(%q): %v", prompt, err)
		}
	}

	var counts []int
	folded := false
	for _, req := range rec.requests() {
		if isSummarizerRequest(req) {
			folded = true
			continue
		}
		counts = append(counts, countUserTag(req, "<workspace>"))
	}
	if !folded {
		t.Fatalf("the fixture never compacted; counts=%v", counts)
	}
	if len(counts) < 2 {
		t.Fatalf("too few agent requests to observe a fold: %v", counts)
	}

	peak, final := 0, counts[len(counts)-1]
	for _, n := range counts {
		if n > peak {
			peak = n
		}
	}
	if final >= peak {
		t.Fatalf("standing-state copies never fell across a fold (peak %d, final %d): superseded copies are riding every fold forever.\ncounts=%v",
			peak, final, counts)
	}
	// The live turn always restates it, so the model is never left without one.
	if final == 0 {
		t.Fatalf("the last request carried no standing state at all; the live turn lost its copy.\ncounts=%v", counts)
	}
}
