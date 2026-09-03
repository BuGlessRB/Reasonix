package agent

import (
	"context"
	"slices"
	"testing"

	"reasonix/internal/agentgraph"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/testenv"
	"reasonix/internal/tool"
)

func graphOf(t *testing.T, run func(ctx context.Context, sink event.Sink)) agentgraph.Graph {
	t.Helper()
	var g agentgraph.Graph
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.GraphDelta && e.Graph != nil {
			g.Apply(*e.Graph)
		}
	})
	run(withCallContext(context.Background(), "call-1", sink, nil, false), sink)
	return g
}

func loneTaskTool(t *testing.T) *TaskTool {
	t.Helper()
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
	}}
	reg := tool.NewRegistry()
	reg.Add(fakeReadFileTool{})
	return NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0.0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(mustSubagentStore(t), testenv.TempDir(t), "base", "high").
		WithScheduler(NewSubagentScheduler(4, 4))
}

// A sub-agent ran eight steps and the run graph said "nothing was delegated":
// only fleet and parallel_tasks published nodes, so every other delegation —
// a task, a run_skill, a review capability — left the picture empty.
func TestALoneDelegationReachesTheRunGraph(t *testing.T) {
	task := loneTaskTool(t)
	g := graphOf(t, func(ctx context.Context, _ event.Sink) {
		if _, err := task.RunProfileSpec(ctx, ProfileExecSpec{
			Task:   TaskSpec{Objective: "inspect the change", Description: "review recent risk"},
			Worker: WorkerSpec{Kind: "skill", Name: "review", Profile: "review", SystemPrompt: "sys"},
			Grant:  CapabilityGrant{ReadOnly: true},
		}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if len(g.Nodes) == 0 {
		t.Fatal("a delegation ran and published no graph node at all")
	}
	group, ok := g.Node("call-1")
	if !ok || group.Kind != agentgraph.KindGroup {
		t.Fatalf("no group node for the delegating call: %+v", g.Nodes)
	}
	worker, ok := g.Node(parallelNodeID("call-1", 0))
	if !ok {
		t.Fatalf("no worker node: %+v", g.Nodes)
	}
	if worker.Kind != agentgraph.KindWorker || worker.ParentID != "call-1" {
		t.Fatalf("worker is not a member of the group: %+v", worker)
	}
	// The lane needs a spawn edge to hang the card from, and the card needs a
	// terminal state or it draws as still running after the turn ended.
	if !slices.Contains(g.Edges, agentgraph.Edge{From: "call-1", To: worker.ID, Kind: agentgraph.Spawn}) {
		t.Fatalf("no spawn edge: %+v", g.Edges)
	}
	if !worker.State.Terminal() {
		t.Fatalf("worker state = %q, want a terminal one", worker.State)
	}
	if worker.Label != "review recent risk" || worker.Profile != "review" {
		t.Fatalf("worker card reads %q / %q, want the task and its profile", worker.Label, worker.Profile)
	}
	if worker.Grant != agentgraph.GrantRead {
		t.Fatalf("grant = %q, want read for a read-only sub-agent", worker.Grant)
	}
}

// A fan-out chose its items' ids and kinds. A second declaration from the
// shared runner would redraw a worker as a group of one.
func TestAFanOutItemIsNotRedeclared(t *testing.T) {
	task := loneTaskTool(t)
	g := graphOf(t, func(ctx context.Context, _ event.Sink) {
		if _, err := task.RunProfileSpec(withDeclaredGraphNode(ctx), ProfileExecSpec{
			Task:   TaskSpec{Objective: "one item of a fleet"},
			Worker: WorkerSpec{Kind: "task", Name: "task", SystemPrompt: "sys"},
		}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	if len(g.Nodes) != 0 {
		t.Fatalf("a fan-out item redeclared itself: %+v", g.Nodes)
	}
}
