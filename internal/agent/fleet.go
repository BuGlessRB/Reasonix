package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"reasonix/internal/ablation"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/jobs"
)

const (
	fleetMinTasks = 2
	fleetMaxTasks = 64
)

// FleetTool dispatches multiple profile-aware sub-agent tasks in parallel
// under the session scheduler. Write tasks must predeclare non-overlapping
// write_paths; preflight failure starts nothing.
type FleetTool struct {
	taskTool *TaskTool
}

// NewFleetTool creates a fleet dispatcher that reuses TaskTool infrastructure.
func NewFleetTool(taskTool *TaskTool) *FleetTool {
	return &FleetTool{taskTool: taskTool}
}

func (*FleetTool) Name() string { return "fleet" }

func (*FleetTool) Description() string {
	return "Dispatch 2–64 sub-agent tasks as a small dependency graph and return bounded previews plus stable Subagent references for full-result retrieval from completed persisted children with read_subagent_result. Each item may select a profile, model, effort, tools, write_paths, or read_only, and may declare depends_on to run after other items and start from their final answers (research → implement → review). Tasks with no dependency between them run in parallel and must declare non-overlapping write_paths; ordered tasks may share paths. Omitted write_paths claim the whole workspace, so two or more concurrent writers without paths fail preflight before any task starts. A failed task's dependents are skipped; independent branches keep going unless fail_fast is set. An item may pass adopt_ref instead of a prompt to stand in for running it with an already-completed child's answer, which is how a fleet a restart interrupted is re-issued without paying twice for the items that finished. Background mode returns a fleet job id collectable with wait."
}

func (*FleetTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "tasks":{
    "type":"array",
    "description":"Array of 2–64 sub-tasks to run under the session scheduler.",
    "minItems":2,
    "maxItems":64,
    "items":{
      "type":"object",
      "properties":{
        "prompt":{"type":"string","description":"Task prompt for the sub-agent. Required unless adopt_ref stands in for running this item."},
        "adopt_ref":{"type":"string","description":"A Subagent reference whose already-completed answer stands in for running this item, from list_subagents or an earlier aggregate. The item runs nothing: its answer becomes the dependency payload its dependents open with, exactly as if it had just produced it. Use it to re-issue a fleet a restart interrupted without paying again for the items that finished. Rejected unless the reference is completed and inside this conversation's lineage and workspace. Mutually exclusive with prompt and with every execution field (profile, model, effort, tools, max_steps, write_paths, read_only), since nothing executes. Whether that answer is still valid after what changed since is yours to judge — the host only proves the reference is yours and complete."},
        "id":{"type":"string","description":"Optional stable id for this task, referenced by other tasks' depends_on. Defaults to the 1-based position."},
        "depends_on":{"type":"array","items":{"type":"string"},"description":"Ids of tasks that must complete before this one starts. This task begins with each dependency's final answer in its context, bounded and split evenly across them, so it need not re-derive their work. Unknown ids, self-edges, and cycles fail preflight. A task whose dependency fails or is skipped is skipped too. Ordered tasks may share write_paths; only tasks that can run at the same time need disjoint claims."},
        "description":{"type":"string","description":"Optional short label shown in the job list."},
        "profile":{"type":"string","description":"Optional runAs=subagent profile name."},
        "write_paths":{"type":"array","items":{"type":"string"},"description":"Write targets for this item. Writers that can run at the same time must declare non-overlapping paths; writers ordered by depends_on may share them. Omitting write_paths claims the whole workspace; two concurrent whole-workspace claims (or any overlap between concurrent writers) fail preflight and start nothing."},
        "read_only":{"type":"boolean","description":"Force the read-only registry even if the profile is writable."},
        "tools":{"type":"array","items":{"type":"string"},"description":"Optional tool whitelist (intersected with profile allowed-tools)."},
        "max_steps":{"type":"integer","description":"Optional max tool-call rounds.","minimum":1},
        "model":{"type":"string","description":"Optional model override."},
        "effort":{"type":"string","description":"Optional reasoning effort override."}
      }
    }
  },
  "fail_fast":{"type":"boolean","description":"Stop starting new tasks after the first failure. Tasks already running are left to finish so partial writes are not abandoned mid-flight. Omitted (the default) means independent branches keep going; a failed task's dependents are skipped either way."},
  "run_in_background":{"type":"boolean","description":"Run the whole fleet asynchronously and return a job id collectable with wait. Items queue for concurrency/write slots inside the job."}
},
"required":["tasks"]
}`)
}

func (*FleetTool) ReadOnly() bool { return false }

type fleetTaskItem struct {
	Prompt      string   `json:"prompt"`
	AdoptRef    string   `json:"adopt_ref"`
	ID          string   `json:"id"`
	DependsOn   []string `json:"depends_on"`
	Description string   `json:"description"`
	Profile     string   `json:"profile"`
	WritePaths  []string `json:"write_paths"`
	ReadOnly    bool     `json:"read_only"`
	Tools       []string `json:"tools"`
	MaxSteps    int      `json:"max_steps"`
	Model       string   `json:"model"`
	Effort      string   `json:"effort"`
}

type fleetItemStatus string

const (
	fleetItemPending   fleetItemStatus = "pending"
	fleetItemCompleted fleetItemStatus = "completed"
	fleetItemAdopted   fleetItemStatus = "adopted"
	fleetItemFailed    fleetItemStatus = "failed"
	fleetItemCancelled fleetItemStatus = "cancelled"
	fleetItemSkipped   fleetItemStatus = "skipped"
)

// answered reports whether an item holds a final answer a dependent can start
// from: one it produced, or one adopted in its place.
func (s fleetItemStatus) answered() bool {
	return s == fleetItemCompleted || s == fleetItemAdopted
}

type fleetItemResult struct {
	index   int
	status  fleetItemStatus
	profile string
	output  string
	err     error
	ref     string
}

// fleetGroupTerminalPhase classifies a fleet group's single terminal status:
// cancellation/deadline wins, then any failed child, then any error
// (including validation failures), then completed.
func fleetGroupTerminalPhase(ctx context.Context, err error, results []fleetItemResult) subagentProgressPhase {
	if ctx.Err() != nil {
		return subagentPhaseCancelled
	}
	for _, r := range results {
		if r.status == fleetItemFailed {
			return subagentPhaseFailed
		}
	}
	if err != nil {
		return subagentPhaseFailed
	}
	return subagentPhaseCompleted
}

func (f *FleetTool) Execute(ctx context.Context, args json.RawMessage) (result string, err error) {
	if f == nil || f.taskTool == nil {
		return "", fmt.Errorf("fleet is not configured")
	}
	// Group lifecycle: the group card's terminal is an explicit event from
	// the tool (running once children start, exactly one terminal at the
	// end) so frontends never infer group completion from the children they
	// happen to have observed. Validation failures emit a failed terminal;
	// once runFleet starts it owns the lifecycle (the background job runs
	// runFleet inside the job, after this function has returned).
	groupParentID, groupSink, _, ok := CallContext(ctx)
	if !ok || groupSink == nil {
		groupParentID = "fleet"
		groupSink = event.Discard
	}
	// The merger emits already-namespaced group/child IDs, so it must use the
	// raw call sink. A nested subSink would prefix the group ID a second time
	// (group/group), leaving the frontend unable to match its lifecycle card.
	merger := newSubagentProgressMerger(realProgressClock{}, groupSink, groupParentID)
	lifecycleHandoff := false
	mergerCloseHandoff := false
	defer func() {
		if !mergerCloseHandoff {
			merger.Close()
		}
	}()
	defer func() {
		if lifecycleHandoff {
			return
		}
		merger.directStatus(groupParentID, fleetGroupTerminalPhase(ctx, err, nil))
	}()
	ctx = withSubagentProgressMerger(ctx, merger)

	var params struct {
		Tasks           []fleetTaskItem `json:"tasks"`
		FailFast        bool            `json:"fail_fast"`
		RunInBackground bool            `json:"run_in_background"`
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&params); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if n := len(params.Tasks); n < fleetMinTasks || n > fleetMaxTasks {
		return "", fmt.Errorf("fleet requires between %d and %d tasks (got %d)", fleetMinTasks, fleetMaxTasks, n)
	}

	for i, item := range params.Tasks {
		if err := validateFleetItemShape(i, item); err != nil {
			return "", err
		}
	}
	plan, err := newFleetPlan(params.Tasks, params.FailFast)
	if err != nil {
		return "", fmt.Errorf("fleet preflight: %w", err)
	}
	adopted, err := f.resolveAdoptions(ctx, params.Tasks)
	if err != nil {
		return "", fmt.Errorf("fleet preflight: %w", err)
	}
	specs, claims, err := f.buildItemSpecs(ctx, params.Tasks, adopted)
	if err != nil {
		return "", err
	}
	if err := plan.validateConcurrentWriteClaims(claims); err != nil {
		return "", fmt.Errorf("fleet preflight: %w", err)
	}

	if params.RunInBackground {
		for i := range specs {
			_, isAdopted := adopted[i]
			specs[i].Sched.BackgroundWriter = !isAdopted && !specs[i].Grant.ReadOnly
		}
		jm, ok := jobs.FromContext(ctx)
		if !ok {
			return "", fmt.Errorf("background execution is not available in this context")
		}
		parentID := groupParentID
		parentSession := ParentSession(ctx)
		label := fmt.Sprintf("fleet(%d)", len(specs))
		backgroundEvidence := evidence.NewLedger()
		writerID := fmt.Sprintf("background-fleet:%s:%d", parentID, time.Now().UnixNano())
		writerRegistered := false
		observer := f.taskTool.mutationObserver
		if observer != nil {
			hasWriter := false
			for i := range specs {
				if specs[i].Sched.BackgroundWriter {
					hasWriter = true
					break
				}
			}
			if hasWriter {
				if err := observer.RegisterWriter(writerID, "background_fleet", observer.OwnershipTurn()); err != nil {
					return "", err
				}
				writerRegistered = true
			}
		}
		job := jm.StartForSession(jobs.SessionFromContext(ctx), "fleet", label, func(jobCtx context.Context, _ io.Writer) (string, error) {
			// Execute returns as soon as the job is registered, so the job owns
			// the handed-off merger until every child preview and terminal has
			// flushed. Closing it in Execute would strand child cards at running.
			defer merger.Close()
			if writerRegistered {
				defer observer.UnregisterWriter(writerID)
			}
			jobCtx = WithParentSession(jobCtx, parentSession)
			jobCtx = evidence.WithLedger(jobCtx, backgroundEvidence)
			defer func() { jobs.PublishEvidence(jobCtx, backgroundEvidence.Summary()) }()
			// The job shares the Execute-level merger so the group lifecycle
			// events and the child previews ride the same pacing budget.
			jobCtx = withSubagentProgressMerger(jobCtx, merger)
			return f.runFleet(jobCtx, groupSink, specs, plan, adopted, parentID)
		})
		// runFleet (inside the job) owns the terminal and merger close from
		// here on. Foreground runFleet hands off only the terminal; Execute
		// still closes the merger after the synchronous call returns.
		lifecycleHandoff = true
		mergerCloseHandoff = true
		return fmt.Sprintf("Started background fleet %q (%s). Collect results with wait; you will be notified when it finishes.", job.ID, label), nil
	}

	lifecycleHandoff = true
	return f.runFleet(ctx, groupSink, specs, plan, adopted, groupParentID)
}

// buildItemSpecs compiles one ProfileExecSpec per runnable item. An adopted item
// gets none: nothing constructs a child for it, which is also why it holds no
// write claim and cannot collide with a concurrent writer.
func (f *FleetTool) buildItemSpecs(ctx context.Context, items []fleetTaskItem, adopted map[int]adoptedItem) ([]ProfileExecSpec, []WritePathSet, error) {
	specs := make([]ProfileExecSpec, len(items))
	// Keep one claim slot per original task so preflight errors report the
	// caller-visible task numbers even when read-only items are interleaved.
	claims := make([]WritePathSet, len(items))
	for i, item := range items {
		if _, isAdopted := adopted[i]; isAdopted {
			continue
		}
		// Fleet writers without write_paths claim the whole workspace so the
		// preflight can detect multi-writer collisions before anything starts.
		forceBackgroundClaim := !item.ReadOnly
		spec, err := f.taskTool.buildTaskSpec(ctx, item.Prompt, item.Description, item.Profile, item.WritePaths, item.Tools, item.MaxSteps, item.Model, item.Effort, "", "", false, item.ReadOnly)
		if err != nil {
			return nil, nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		if forceBackgroundClaim && !spec.Grant.ReadOnly && spec.Grant.WritePaths.Empty() {
			whole, werr := WholeWorkspaceWriteClaim(f.taskTool.workspaceRoot)
			if werr != nil {
				return nil, nil, fmt.Errorf("task %d: %w", i+1, werr)
			}
			spec.Grant.WritePaths = whole
		}
		spec.Sched.Nested = SubagentDepth(ctx) > 0
		spec.Sched.RunInBackground = false // fleet owns backgrounding
		if spec.Task.Description == "" {
			spec.Task.Description = fmt.Sprintf("fleet-%d", i+1)
		}
		specs[i] = spec
		if !spec.Grant.ReadOnly {
			claims[i] = spec.Grant.WritePaths
		}
	}
	return specs, claims, nil
}

func (f *FleetTool) runFleet(ctx context.Context, sink event.Sink, specs []ProfileExecSpec, plan fleetPlan, adopted map[int]adoptedItem, groupParentID string) (result string, err error) {
	if sink == nil {
		sink = event.Discard
	}
	// Child IDs are namespaced exactly once under the group call. Background
	// jobs no longer carry the original call context, so groupParentID is the
	// authoritative identity there; direct callers fall back to CallContext.
	parentID := strings.TrimSpace(groupParentID)
	if parentID == "" {
		var ok bool
		parentID, _, _, ok = CallContext(ctx)
		if !ok || parentID == "" {
			parentID = "fleet"
		}
	}
	groupParentID = parentID
	// The Execute-level merger (or a fallback for direct callers) paces the
	// group; runFleet owns the lifecycle once it starts: running up front
	// and exactly one terminal after every child settles.
	merger := subagentProgressMergerFromContext(ctx)
	ownsMerger := false
	if merger == nil {
		merger = newSubagentProgressMerger(realProgressClock{}, sink, groupParentID)
		ownsMerger = true
		ctx = withSubagentProgressMerger(ctx, merger)
	}
	if ownsMerger {
		defer merger.Close()
	}
	merger.directStatus(groupParentID, subagentPhaseRunning)
	var results []fleetItemResult
	defer func() {
		merger.directStatus(groupParentID, fleetGroupTerminalPhase(ctx, err, results))
	}()

	n := len(specs)
	results = make([]fleetItemResult, n)
	for i := range results {
		results[i] = fleetItemResult{index: i, status: fleetItemPending, profile: specs[i].Worker.Profile}
		// An adopted node enters already resolved, so the driver never launches
		// it and its dependents are ready from the first pass.
		if a, ok := adopted[i]; ok {
			results[i] = fleetItemResult{index: i, status: fleetItemAdopted, output: a.answer, ref: a.ref}
		}
	}

	var wg sync.WaitGroup
	doneCh := make(chan fleetItemResult, n)

	startOne := func(idx int) {
		spec := specs[idx]
		spec.Context.Upstream = f.upstreamFor(plan, idx, results)
		label := spec.Task.Description
		subID := fmt.Sprintf("%s/fleet-%d", parentID, idx+1)
		dispatchArgs, _ := json.Marshal(map[string]any{
			"prompt":      spec.Task.Objective,
			"description": label,
			"profile":     spec.Worker.Profile,
		})
		sink.Emit(event.Event{
			Kind: event.ToolDispatch,
			Tool: event.Tool{
				ID: subID, ParentID: parentID, Name: "task",
				Args: string(dispatchArgs), ReadOnly: spec.Grant.ReadOnly,
			},
		})

		wg.Go(func() {
			// Each fleet item runs as its own task-shaped execution so
			// transcripts, evidence, and scheduler claims stay independent.
			itemCtx := withCallContext(ctx, subID, subSinkFor(subID, sink), nil, false)
			out, err := f.taskTool.RunProfileSpec(itemCtx, spec)
			answer, ref := splitSubagentRunResult(out)
			res := fleetItemResult{index: idx, profile: spec.Worker.Profile, output: answer, ref: ref, err: err}
			if err == nil {
				res.status = fleetItemCompleted
				sink.Emit(event.Event{
					Kind: event.ToolResult,
					Tool: event.Tool{ID: subID, ParentID: parentID, Name: "task", Output: out},
				})
			} else {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					res.status = fleetItemCancelled
				} else {
					res.status = fleetItemFailed
				}
				sink.Emit(event.Event{
					Kind: event.ToolResult,
					Tool: event.Tool{ID: subID, ParentID: parentID, Name: "task", Err: err.Error()},
				})
			}
			doneCh <- res
		})
	}

	cancelled := driveFleet(ctx, plan, results, doneCh, wg.Wait, startOne)
	for _, r := range results {
		if r.status == fleetItemCancelled || r.status == fleetItemSkipped {
			cancelled = true
			break
		}
	}
	if cancelled {
		err := ctx.Err()
		if err == nil {
			err = context.Canceled
		}
		return formatFleetAggregate(results, true), err
	}
	return formatFleetAggregate(results, false), nil
}

// upstreamFor is the plan's delivery with the measurement arm applied. The
// no-upstream arm orders the endpoints and delivers nothing, so a benchmark can
// attribute a result to the data the edge carries rather than to its order.
func (f *FleetTool) upstreamFor(plan fleetPlan, idx int, results []fleetItemResult) []UpstreamResult {
	if f.taskTool != nil && f.taskTool.ablation.Off(ablation.Upstream) {
		return nil
	}
	return plan.upstreamFor(idx, results)
}

func formatFleetAggregate(results []fleetItemResult, cancelled bool) string {
	n := len(results)
	var prefix string
	if cancelled {
		completed := 0
		for _, r := range results {
			if r.status.answered() {
				completed++
			}
		}
		prefix = fmt.Sprintf("Cancelled fleet after completing %d of %d tasks:\n", completed, n)
	} else {
		prefix = fmt.Sprintf("Completed fleet of %d tasks:\n", n)
	}
	items := make([]subagentAggregateItem, 0, n)
	for i, r := range results {
		header := fmt.Sprintf("── task-%d", i+1)
		if r.profile != "" {
			header += " profile=" + boundedInline(r.profile, 80)
		}
		header += " ──\n"
		item := subagentAggregateItem{header: header, ref: r.ref}
		switch r.status {
		case fleetItemCompleted:
			item.status = "status: completed\n"
			item.answer = strings.TrimSpace(r.output)
		case fleetItemAdopted:
			item.status = "status: adopted (not run; the answer below is the reference's)\n"
			item.answer = strings.TrimSpace(r.output)
		case fleetItemFailed:
			item.status = "status: failed\n"
			if r.err != nil {
				item.detail = fmt.Sprintf("[FAILED] %s\n", boundedInline(r.err.Error(), 256))
			}
		case fleetItemCancelled:
			item.status = "status: cancelled\n"
			if r.err != nil {
				item.detail = fmt.Sprintf("[CANCELLED] %s\n", boundedInline(r.err.Error(), 256))
			}
		case fleetItemSkipped:
			item.status = "status: skipped\n"
			if r.err != nil {
				item.detail = fmt.Sprintf("[SKIPPED] %s\n", boundedInline(r.err.Error(), 256))
			}
		default:
			item.status = "status: pending\n"
		}
		items = append(items, item)
	}
	return formatBoundedSubagentAggregate(prefix, items)
}
