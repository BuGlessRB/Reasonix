package main

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/ablation"
	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// A fixture is a real session that has really been compacted: no bench-only
// state shape, so the loader and the projection restore have to work on it.

const (
	fixtureWindow      = 60000
	fixtureGenerations = 5
	// fillerBytes per unit is what makes a fold necessary rather than optional.
	fillerBytes = 6000
)

// digestProvider answers a summarizer without naming anything the corpus
// plants. A digest that happened to quote the answer would leave it visible
// and silently turn the task into a no-op.
type digestProvider struct{ calls int }

func (p *digestProvider) Name() string { return "fixture-digest" }

func (p *digestProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.calls++
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "Earlier work continued across several files; nothing outstanding was recorded."}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// fillerCall is one ordinary read whose only job is to take up room — in the
// window, and in the fold index ahead of a cue.
func fillerCall(sess *agent.Session, n int) {
	id := fmt.Sprintf("f%04d", n)
	sess.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
		{ID: id, Name: "read_file", Arguments: fmt.Sprintf(`{"path":"internal/mod%03d/handler%03d.go"}`, n%37, n)},
	}})
	sess.Add(provider.Message{Role: provider.RoleTool, ToolCallID: id, Name: "read_file", Content: fillerBody(n)})
}

func fillerBody(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package mod%03d\n\n", n%37)
	for b.Len() < fillerBytes {
		fmt.Fprintf(&b, "func handler%03dStep%02d() error { return nil }\n", n, b.Len()%64)
	}
	return b.String()
}

// builtFixture is one task's history after real folds, plus where its target
// ended up.
type builtFixture struct {
	Session *agent.Session
	State   agent.CompactionState
	Target  int
	Path    string
}

// buildFixture plays a task's history through the real agent and folds it at
// the given arm, writing the session and its context state where the agent
// itself would.
func buildFixture(t contextTask, arm ablation.Set, sessionPath string) (builtFixture, error) {
	sess := agent.NewSession("You are a coding agent.")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "Work through the transport and scheduler backlog."})

	n := 0
	at := -1

	a := agent.New(&digestProvider{}, tool.NewRegistry(), sess, agent.Options{
		ContextWindow: fixtureWindow, CompactRatio: 0.5, RecentKeep: 2,
		SessionPath: sessionPath, KeepPolicy: agent.KeepErrors, Ablation: arm,
	}, event.Discard)

	for gen := range fixtureGenerations {
		if gen == t.PlantAfterGen {
			at = t.Plant(sess)
		}
		for range 8 {
			fillerCall(sess, n)
			n++
		}
		if err := a.CompactNow(context.Background(), ""); err != nil {
			return builtFixture{}, fmt.Errorf("%s: generation %d fold: %w", t.ID, gen, err)
		}
	}
	if at < 0 {
		return builtFixture{}, fmt.Errorf("%s: PlantAfterGen %d is past the last generation", t.ID, t.PlantAfterGen)
	}
	// projectionCoversTail cannot revalidate a projection covering the whole
	// transcript: it needs a live tail, which a real session always has because
	// work continues after a fold. Neutral text — the tail is provider-visible.
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "Backlog swept; ready for the next question."})
	if err := sess.Save(sessionPath); err != nil {
		return builtFixture{}, fmt.Errorf("%s: save session: %w", t.ID, err)
	}
	state, ok, err := agent.LoadCompactionState(sessionPath)
	if err != nil {
		return builtFixture{}, fmt.Errorf("%s: load context state: %w", t.ID, err)
	}
	if !ok || len(state.Projection.Messages) == 0 {
		return builtFixture{}, fmt.Errorf("%s: no projection was installed", t.ID)
	}
	if state.Projection.CoveredCount <= at {
		return builtFixture{}, fmt.Errorf("%s: target #%d is not inside the folded region (covered %d)",
			t.ID, at, state.Projection.CoveredCount)
	}
	return builtFixture{Session: sess, State: state, Target: at, Path: sessionPath}, nil
}

// visibleContext is the model-visible view: the stored projection spliced with
// everything canonical that came after it.
func visibleContext(f builtFixture) []provider.Message {
	canonical := f.Session.Snapshot()
	out := append([]provider.Message(nil), f.State.Projection.Messages...)
	if n := f.State.Projection.CoveredCount; n >= 0 && n < len(canonical) {
		out = append(out, canonical[n:]...)
	}
	return out
}

// visibleText is everything the model would read, as one string.
func visibleText(f builtFixture) string {
	var b strings.Builder
	for _, m := range visibleContext(f) {
		b.WriteString(m.Content)
		b.WriteString("\n")
		for _, tc := range m.ToolCalls {
			b.WriteString(tc.Name)
			b.WriteString(" ")
			b.WriteString(string(tc.Arguments))
			b.WriteString("\n")
		}
	}
	return b.String()
}
