package agent

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"reasonix/internal/ablation"
	"reasonix/internal/agentgraph"
	"reasonix/internal/event"
	"reasonix/internal/tool"
)

// foldGraph replays the published deltas the way every consumer does, so these
// tests assert the graph a frontend ends up holding rather than the wording of
// the events that built it.
func foldGraph(t *testing.T, sink *recordSink) agentgraph.Graph {
	t.Helper()
	var g agentgraph.Graph
	for _, e := range sink.kinds(event.GraphDelta) {
		if e.Graph == nil {
			t.Fatal("a graph_delta event carried no delta")
		}
		g.Apply(*e.Graph)
	}
	return g
}

func requireEdge(t *testing.T, g agentgraph.Graph, from, to string, kind agentgraph.EdgeKind) {
	t.Helper()
	if !slices.Contains(g.Edges, agentgraph.Edge{From: from, To: to, Kind: kind}) {
		t.Fatalf("missing %s edge %s to %s in %v", kind, from, to, g.Edges)
	}
}

func requireState(t *testing.T, g agentgraph.Graph, id string, want agentgraph.NodeState) {
	t.Helper()
	node, ok := g.Node(id)
	if !ok {
		t.Fatalf("no node %q in %+v", id, g.Nodes)
	}
	if node.State != want {
		t.Fatalf("node %q state = %q, want %q", id, node.State, want)
	}
}

func newProbeFleet(t *testing.T, prov *upstreamProbeProvider, arm ablation.Set) *FleetTool {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(fakeReadFileTool{})
	return NewFleetTool(NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0.0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(mustSubagentStore(t), t.TempDir(), "base", "high").
		WithScheduler(NewSubagentScheduler(4, 4)).
		WithAblation(arm))
}

// The dependency graph preflight proved used to die with the call that built
// it: the dispatch carried a prompt and a profile, and nothing downstream could
// tell an item waiting on a result from one that had simply not started.
func TestFleetPublishesTheGraphPreflightProved(t *testing.T) {
	rec := &recordSink{}
	fleet := newProbeFleet(t, &upstreamProbeProvider{answer: "done"}, ablation.New())
	ctx := WithParentSession(withCallContext(context.Background(), "fleet-call", rec, nil, false), "graph-parent")

	out, err := fleet.Execute(ctx, json.RawMessage(fleetGraphTasks))
	if err != nil {
		t.Fatalf("fleet: %v\n%s", err, out)
	}

	g := foldGraph(t, rec)
	const group, research, implement, aside = "fleet-call", "fleet-call/fleet-1", "fleet-call/fleet-2", "fleet-call/fleet-3"
	for _, id := range []string{group, research, implement, aside} {
		requireState(t, g, id, agentgraph.StateCompleted)
	}
	for _, id := range []string{research, implement, aside} {
		requireEdge(t, g, group, id, agentgraph.Spawn)
	}
	// The edge this whole change is about: it never left the process before.
	requireEdge(t, g, research, implement, agentgraph.Depends)
	ordered := func(e agentgraph.Edge) bool { return e.Kind == agentgraph.Depends && e.To == aside }
	if slices.ContainsFunc(g.Edges, ordered) {
		t.Fatalf("an independent item was drawn as ordered: %v", g.Edges)
	}

	node, _ := g.Node(implement)
	if node.Label != "rewrite it" || node.Kind != agentgraph.KindWorker || node.Grant != agentgraph.GrantWrite {
		t.Fatalf("worker node = %+v, want this item's own label and grant", node)
	}
	if node.Ref == "" {
		t.Fatalf("a settled node must name the transcript its answer reads back from: %+v", node)
	}
	if root, _ := g.Node(group); root.Kind != agentgraph.KindGroup {
		t.Fatalf("group node = %+v, want kind group", root)
	}
}

// Ordering and delivery are two facts, and the measurement arm is what proves
// it: the same pair stays ordered while nothing travels the edge between them.
func TestFleetGraphTellsOrderingFromDelivery(t *testing.T) {
	const research, implement = "fleet-call/fleet-1", "fleet-call/fleet-2"
	for _, tc := range []struct {
		name        string
		arm         ablation.Set
		wantContext bool
	}{
		{"delivered", ablation.New(), true},
		{"ordered only", ablation.New(ablation.Upstream), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordSink{}
			fleet := newProbeFleet(t, &upstreamProbeProvider{answer: "PROBE"}, tc.arm)
			ctx := WithParentSession(withCallContext(context.Background(), "fleet-call", rec, nil, false), "arm-parent")
			if out, err := fleet.Execute(ctx, json.RawMessage(fleetGraphPairTasks)); err != nil {
				t.Fatalf("fleet: %v\n%s", err, out)
			}
			g := foldGraph(t, rec)
			requireEdge(t, g, research, implement, agentgraph.Depends)
			carried := agentgraph.Edge{From: research, To: implement, Kind: agentgraph.Context}
			if got := slices.Contains(g.Edges, carried); got != tc.wantContext {
				t.Fatalf("context edge present = %v, want %v; edges %v", got, tc.wantContext, g.Edges)
			}
		})
	}
}

// A skipped item dispatches nothing, so its state exists only because the graph
// says so. Without it a reader sees a branch that merely stopped.
func TestFleetGraphRecordsTheBranchAFailureKilled(t *testing.T) {
	rec := &recordSink{}
	reg := tool.NewRegistry()
	reg.Add(fakeReadFileTool{})
	task := NewTaskTool(&fleetAPIErrorProvider{status: 429}, nil, reg, 20, 0, 0, 0, 0.0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(mustSubagentStore(t), t.TempDir(), "base", "high").
		WithScheduler(NewSubagentScheduler(3, 3))
	ctx := WithParentSession(withCallContext(context.Background(), "fleet-call", rec, nil, false), "skip-parent")

	if _, err := NewFleetTool(task).Execute(ctx, json.RawMessage(fleetGraphFailureTasks)); err != nil {
		t.Fatalf("a fleet that ran to its end must not answer an error: %v", err)
	}

	g := foldGraph(t, rec)
	requireState(t, g, "fleet-call/fleet-1", agentgraph.StateFailed)
	requireState(t, g, "fleet-call/fleet-2", agentgraph.StateSkipped)
	requireState(t, g, "fleet-call/fleet-3", agentgraph.StateCompleted)
	requireState(t, g, "fleet-call", agentgraph.StateFailed)
	if head, _ := g.Node("fleet-call/fleet-1"); head.Err == "" {
		t.Fatalf("a failed node must carry why: %+v", head)
	}
}

// Reuse is the one thing a list of running children can never show. An adopted
// item runs nothing and emits no tool events at all, so the external node and
// its edge are the only record that the work was not paid for twice.
func TestFleetGraphShowsAdoptedWorkAsExternal(t *testing.T) {
	root := t.TempDir()
	store := mustSubagentStore(t)
	first := newAdoptionFleet(t, store, root, &upstreamProbeProvider{answer: "ARTIFACT-42"})
	out, err := first.Execute(adoptionCtx(event.Discard), json.RawMessage(fleetGraphSeedTasks))
	if err != nil {
		t.Fatalf("first fleet: %v\n%s", err, out)
	}
	refs := subagentRefsIn(out)
	if len(refs) != 2 {
		t.Fatalf("expected a reference per child, got %v", refs)
	}

	rec := &recordSink{}
	second := newAdoptionFleet(t, store, root, &upstreamProbeProvider{answer: "SHOULD-NOT-RUN"})
	reissue, err := json.Marshal(map[string]any{"tasks": []map[string]any{
		{"id": "research", "adopt_ref": refs[0]},
		{"id": "implement", "prompt": "TASK-BETA rewrite", "depends_on": []string{"research"}, "write_paths": []string{"parse.go"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out, err = second.Execute(adoptionCtx(rec), reissue); err != nil {
		t.Fatalf("second fleet: %v\n%s", err, out)
	}

	g := foldGraph(t, rec)
	const adopted, dependent = "fleet-call/fleet-1", "fleet-call/fleet-2"
	requireState(t, g, adopted, agentgraph.StateAdopted)
	requireEdge(t, g, refs[0], adopted, agentgraph.Adopt)
	requireEdge(t, g, adopted, dependent, agentgraph.Depends)
	requireEdge(t, g, adopted, dependent, agentgraph.Context)
	source, ok := g.Node(refs[0])
	if !ok || source.Kind != agentgraph.KindExternal {
		t.Fatalf("the adopted answer's source = %+v, want an external node", source)
	}
}

const (
	fleetGraphTasks = `{"tasks":[
		{"id":"research","description":"survey the parser","prompt":"TASK-ALPHA survey","read_only":true},
		{"id":"implement","description":"rewrite it","prompt":"TASK-BETA rewrite","depends_on":["research"],"write_paths":["parse.go"]},
		{"id":"aside","description":"unrelated","prompt":"TASK-GAMMA aside","read_only":true}
	]}`

	fleetGraphPairTasks = `{"tasks":[
		{"id":"research","prompt":"TASK-ALPHA survey","read_only":true},
		{"id":"implement","prompt":"TASK-BETA rewrite","depends_on":["research"],"write_paths":["parse.go"]}
	]}`

	fleetGraphFailureTasks = `{"tasks":[
		{"id":"head","prompt":"FAIL here","read_only":true},
		{"id":"downstream","prompt":"downstream work","depends_on":["head"],"read_only":true},
		{"id":"sibling","prompt":"sibling work","read_only":true}
	]}`

	fleetGraphSeedTasks = `{"tasks":[
		{"id":"research","prompt":"TASK-ALPHA survey","read_only":true},
		{"id":"aside","prompt":"TASK-GAMMA aside","read_only":true}
	]}`
)

// Concurrency is the absence of ordering, and a picture can only show it if
// the graph says these nodes exist and nothing joins them. parallel_tasks is
// where that has to hold: it has no dependencies to draw at all.
func TestParallelTasksPublishesAnUnorderedGroup(t *testing.T) {
	rec := &recordSink{}
	task := newTestTaskTool(t, parallelStaticProvider{}, tool.NewRegistry(), "sys", "", "", nil)
	parallel := NewParallelTasksTool(task, tool.NewRegistry())
	ctx := withCallContext(context.Background(), "parallel-call", rec, nil, false)

	if _, err := parallel.Execute(ctx, json.RawMessage(parallelGraphTasks)); err != nil {
		t.Fatalf("parallel_tasks: %v", err)
	}

	g := foldGraph(t, rec)
	const group, first, second = "parallel-call", "parallel-call/sub-1", "parallel-call/sub-2"
	requireState(t, g, group, agentgraph.StateCompleted)
	requireState(t, g, first, agentgraph.StateCompleted)
	requireState(t, g, second, agentgraph.StateCompleted)
	requireEdge(t, g, group, first, agentgraph.Spawn)
	requireEdge(t, g, group, second, agentgraph.Spawn)
	if node, _ := g.Node(first); node.Label != "survey" || node.Grant != agentgraph.GrantRead {
		t.Fatalf("worker node = %+v, want its label and the read-only grant this tool forces", node)
	}
	for _, e := range g.Edges {
		if e.Kind != agentgraph.Spawn {
			t.Fatalf("a parallel group must publish only structure, got %+v", e)
		}
	}
}

// graphStep is one state a delta declared, in publication order. The folded
// graph keeps only the last one per node, so a lifecycle can be asserted only
// against the stream that built it.
type graphStep struct {
	id    string
	state agentgraph.NodeState
}

func stateStream(t *testing.T, sink *recordSink) []graphStep {
	t.Helper()
	var out []graphStep
	for _, e := range sink.kinds(event.GraphDelta) {
		if e.Graph == nil {
			t.Fatal("a graph_delta event carried no delta")
		}
		for _, n := range e.Graph.Nodes {
			if n.State != "" {
				out = append(out, graphStep{id: n.ID, state: n.State})
			}
		}
	}
	return out
}

func firstAt(stream []graphStep, id string, state agentgraph.NodeState) int {
	return slices.IndexFunc(stream, func(s graphStep) bool { return s.id == id && s.state == state })
}

func lifecycleOf(stream []graphStep, id string) []agentgraph.NodeState {
	var out []agentgraph.NodeState
	for _, s := range stream {
		if s.id == id {
			out = append(out, s.state)
		}
	}
	return out
}

func fleetWorkerIDs() []string {
	return []string{"fleet-call/fleet-1", "fleet-call/fleet-2", "fleet-call/fleet-3"}
}

func runProbeFleet(t *testing.T, session string) *recordSink {
	t.Helper()
	rec := &recordSink{}
	fleet := newProbeFleet(t, &upstreamProbeProvider{answer: "done"}, ablation.New())
	ctx := WithParentSession(withCallContext(context.Background(), "fleet-call", rec, nil, false), session)
	if out, err := fleet.Execute(ctx, json.RawMessage(fleetGraphTasks)); err != nil {
		t.Fatalf("fleet: %v\n%s", err, out)
	}
	return rec
}

// The graph declared three timestamps and no producer ever filled one, so every
// consumer's timing fallback was dead code and a run's own order survived only
// as the order its deltas happened to be published in.
func TestFleetGraphTimesTheRunItDraws(t *testing.T) {
	g := foldGraph(t, runProbeFleet(t, "clock-parent"))
	for _, id := range fleetWorkerIDs() {
		n, ok := g.Node(id)
		if !ok {
			t.Fatalf("no node %q in %+v", id, g.Nodes)
		}
		if n.QueuedAt == 0 || n.StartedAt == 0 || n.EndedAt == 0 {
			t.Fatalf("node %q = %+v, want every stamp of a run that actually happened", id, n)
		}
		if n.QueuedAt > n.StartedAt || n.StartedAt > n.EndedAt {
			t.Fatalf("node %q runs backwards: queued %d, started %d, ended %d", id, n.QueuedAt, n.StartedAt, n.EndedAt)
		}
	}
	group, ok := g.Node("fleet-call")
	if !ok || group.StartedAt == 0 || group.EndedAt < group.StartedAt {
		t.Fatalf("group node = %+v, want the span the whole fan-out sits inside", group)
	}
}

// Waiting for a slot and holding one were the same word, so a fan-out the
// session concurrency ceiling was throttling drew identically to one where every
// item started at once. Which of the two it is decides whether raising the
// ceiling would buy anything, and nothing else in the run can answer it.
func TestFleetGraphSaysQueuedBeforeItSaysRunning(t *testing.T) {
	stream := stateStream(t, runProbeFleet(t, "queue-parent"))
	for _, id := range fleetWorkerIDs() {
		queued := firstAt(stream, id, agentgraph.StateQueued)
		running := firstAt(stream, id, agentgraph.StateRunning)
		if queued < 0 || running < 0 {
			t.Fatalf("node %q lifecycle = %v, want queued then running", id, lifecycleOf(stream, id))
		}
		if queued > running {
			t.Fatalf("node %q held a slot before it asked for one: %v", id, lifecycleOf(stream, id))
		}
	}
}

// An item's outcome used to reach the graph only when its group ended, so a
// fleet drew every finished worker as still running until its slowest sibling
// returned — which on a long fan-out is the whole time anyone is watching it.
func TestFleetGraphSettlesAnItemBeforeTheGroupDoes(t *testing.T) {
	stream := stateStream(t, runProbeFleet(t, "settle-parent"))
	groupEnd := firstAt(stream, "fleet-call", agentgraph.StateCompleted)
	if groupEnd < 0 {
		t.Fatalf("the group never settled: %v", lifecycleOf(stream, "fleet-call"))
	}
	for _, id := range fleetWorkerIDs() {
		at := firstAt(stream, id, agentgraph.StateCompleted)
		if at < 0 || at > groupEnd {
			t.Fatalf("item %q reached the graph at step %d and the group ended at %d; a finished item must not wait for its siblings", id, at, groupEnd)
		}
	}
}

// parallel_tasks has no dependencies at all, so a slot is the only thing one of
// its items can ever be waiting for. That makes it the case where saying
// "running" from the moment of dispatch is furthest from true.
func TestParallelTasksGraphSeparatesQueuedFromRunning(t *testing.T) {
	rec := &recordSink{}
	task := newTestTaskTool(t, parallelStaticProvider{}, tool.NewRegistry(), "sys", "", "", nil)
	parallel := NewParallelTasksTool(task, tool.NewRegistry())
	ctx := withCallContext(context.Background(), "parallel-call", rec, nil, false)
	if _, err := parallel.Execute(ctx, json.RawMessage(parallelGraphTasks)); err != nil {
		t.Fatalf("parallel_tasks: %v", err)
	}

	stream := stateStream(t, rec)
	groupEnd := firstAt(stream, "parallel-call", agentgraph.StateCompleted)
	g := foldGraph(t, rec)
	for _, id := range []string{"parallel-call/sub-1", "parallel-call/sub-2"} {
		queued := firstAt(stream, id, agentgraph.StateQueued)
		running := firstAt(stream, id, agentgraph.StateRunning)
		settled := firstAt(stream, id, agentgraph.StateCompleted)
		if queued < 0 || running < 0 || queued > running {
			t.Fatalf("node %q lifecycle = %v, want queued then running", id, lifecycleOf(stream, id))
		}
		if settled < 0 || groupEnd < 0 || settled > groupEnd {
			t.Fatalf("node %q settled at %d and the group ended at %d", id, settled, groupEnd)
		}
		n, _ := g.Node(id)
		if n.QueuedAt == 0 || n.StartedAt < n.QueuedAt || n.EndedAt < n.StartedAt {
			t.Fatalf("node %q = %+v, want a queued/started/ended span in order", id, n)
		}
	}
}

const parallelGraphTasks = `{"tasks":[
	{"prompt":"first","description":"survey"},
	{"prompt":"second","description":"measure"}
]}`
