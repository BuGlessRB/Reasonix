package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"reasonix/internal/event"
	"reasonix/internal/tool"
)

const (
	// fanoutHardMaxItems bounds any single fan-out regardless of what the call
	// asked for. A node whose width the caller cannot predict still cannot
	// spend without a ceiling the host owns.
	fanoutHardMaxItems = 32
	// fanoutDefaultMaxItems applies when the call names no ceiling of its own.
	fanoutDefaultMaxItems = 8
	// fanoutMaxItemBytes bounds one item. An item is a subject to work on, not
	// a payload; a long one is a sign the source answered in prose.
	fanoutMaxItemBytes = 512
)

// fanoutSink collects what a fan-out source submits. It is per source run and
// travels on that run's context, the same way its write claim and its upstream
// answers do.
type fanoutSink struct {
	mu       sync.Mutex
	items    []string
	received bool
}

func (s *fanoutSink) submit(items []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = items
	s.received = true
}

// collected returns the submitted items and whether the source submitted at
// all. The two are different outcomes: nothing to work on is a result, and
// never answering is a source that did not do its job.
func (s *fanoutSink) collected() ([]string, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.items...), s.received
}

type fanoutSinkKey struct{}

func withFanoutSink(ctx context.Context, sink *fanoutSink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, fanoutSinkKey{}, sink)
}

func fanoutSinkFromContext(ctx context.Context) *fanoutSink {
	sink, _ := ctx.Value(fanoutSinkKey{}).(*fanoutSink)
	return sink
}

// SubmitItemsTool is how a fan-out source names the subjects the mapped node
// will run over. It exists so the host never has to read a list out of prose:
// the caller fixed the template and the width ceiling at preflight, and this
// supplies only which subjects, which is data rather than plan.
type SubmitItemsTool struct{}

func (*SubmitItemsTool) Name() string { return "submit_items" }

func (*SubmitItemsTool) Description() string {
	return "Submit the subjects the mapped task should run over, one per entry, as this run's structured result. Call it once. An entry names a single subject — a path, an identifier, a symbol — not a description of it and not your reasoning about it. Submitting an empty list is a real answer: it means there is nothing to work on."
}

// ReadOnly: naming subjects changes no workspace state.
func (*SubmitItemsTool) ReadOnly() bool { return true }

func (*SubmitItemsTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"string"},"description":"The subjects to run the mapped task over, one per entry. Each names a single subject and nothing else."}},"required":["items"]}`)
}

func (*SubmitItemsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Items []string `json:"items"`
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	sink := fanoutSinkFromContext(ctx)
	if sink == nil {
		return "", fmt.Errorf("submit_items belongs to a fan-out source; nothing in this run maps over its result")
	}
	items, err := normalizeFanoutItems(p.Items)
	if err != nil {
		return "", err
	}
	sink.submit(items)
	if len(items) == 0 {
		return "submit_items accepted: no subjects, so the mapped task runs nothing.", nil
	}
	return fmt.Sprintf("submit_items accepted: %d subject(s).", len(items)), nil
}

// normalizeFanoutItems trims, drops blanks, and rejects duplicates and prose.
// A duplicate would run the same work twice under one node and report it as two
// findings, which is worse than refusing it.
func normalizeFanoutItems(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len(item) > fanoutMaxItemBytes {
			return nil, fmt.Errorf("item %q exceeds %d bytes; an entry names one subject, it does not describe it", boundedInline(item, 80), fanoutMaxItemBytes)
		}
		if seen[item] {
			return nil, fmt.Errorf("item %q is submitted twice; the mapped task would run it twice and report it as two results", boundedInline(item, 80))
		}
		seen[item] = true
		out = append(out, item)
	}
	if len(out) > fanoutHardMaxItems {
		return nil, fmt.Errorf("%d items exceeds the %d the host will map over in one node; narrow the subjects", len(out), fanoutHardMaxItems)
	}
	return out, nil
}

// attachFanoutTool adds submit_items to a child registry only when this run is
// a fan-out source. Every other child keeps the tool off its surface, so a
// capability nothing in the run can use never reaches its prompt.
func attachFanoutTool(ctx context.Context, reg *tool.Registry) {
	if reg == nil || fanoutSinkFromContext(ctx) == nil {
		return
	}
	reg.Add(&SubmitItemsTool{})
}

// fanoutSourceContract is appended to a fan-out source's task text. The source
// still answers in prose for the transcript; this states the one thing the host
// reads structurally.
const fanoutSourceContract = `<fanout-contract>
Another task in this call maps over your result, so the subjects you find must be
submitted with submit_items, exactly once, as a list of bare names. Prose in your
final answer is not read for them. An empty list is a valid submission and means
there is nothing to map over.
</fanout-contract>`

// fanoutItemObjective is the mapped template applied to one subject. The
// subject is task text rather than starting context: what this child is asked
// to do is the template *about this subject*, and delivery classification
// should judge exactly that.
func fanoutItemObjective(template, item string) string {
	return strings.TrimSpace(template) + "\n\nThe subject for this run: " + item
}

// resolvedFanoutWidth reports how many subjects a node may map over, given what
// the call asked for.
func resolvedFanoutWidth(requested int) int {
	if requested <= 0 {
		return fanoutDefaultMaxItems
	}
	return min(requested, fanoutHardMaxItems)
}

// fleetDispatch is what the driver needs beyond the graph: which nodes stand
// down to a prior answer, and how wide a mapped node may run. Both are settled
// at preflight; neither changes the shape the plan validated.
type fleetDispatch struct {
	adopted map[int]adoptedItem
	widths  []int
}

// withoutUpstreamFrom drops one dependency's answer from a delivery.
func withoutUpstreamFrom(results []UpstreamResult, id string) []UpstreamResult {
	out := make([]UpstreamResult, 0, len(results))
	for _, r := range results {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return out
}

// validateFanoutSources rejects a source that cannot supply subjects. Both cases
// are silent failures otherwise: an adopted node never ran, so nothing called
// submit_items for it, and a mapped node answers with an aggregate rather than a
// list.
func validateFanoutSources(items []fleetTaskItem, plan fleetPlan, adopted map[int]adoptedItem) error {
	for i := range items {
		src := plan.fanoutSource[i]
		if src < 0 {
			continue
		}
		if _, isAdopted := adopted[src]; isAdopted {
			return fmt.Errorf("%s maps over %q, which is adopted and so never ran to submit subjects", plan.describe(i), plan.ids[src])
		}
		if plan.fanoutSource[src] >= 0 {
			return fmt.Errorf("%s maps over %q, which is itself a mapped task and answers with an aggregate rather than a list of subjects", plan.describe(i), plan.ids[src])
		}
	}
	return nil
}

// runFanout maps one node's template over the subjects its source submitted.
// The graph never learns of these children: the node stays a single vertex, so
// the reachability, write claims, and skip rules preflight validated are the
// ones that actually ran. What is dynamic is the node's width, never the shape.
func (f *FleetTool) runFanout(ctx context.Context, sink event.Sink, parentID string, idx int, spec ProfileExecSpec, width int, src *fanoutSink) fleetItemResult {
	res := fleetItemResult{index: idx, profile: spec.Worker.Profile}
	subjects, answered := src.collected()
	if !answered {
		res.status = fleetItemFailed
		res.err = fmt.Errorf("the fan-out source finished without calling submit_items, so there are no subjects to map over")
		return res
	}
	if len(subjects) > width {
		res.status = fleetItemFailed
		res.err = fmt.Errorf("the source submitted %d subjects, above this task's ceiling of %d; raise max_items or narrow the source", len(subjects), width)
		return res
	}
	if len(subjects) == 0 {
		res.status = fleetItemCompleted
		res.output = "No subjects to map over: the source submitted an empty list."
		return res
	}

	template := spec.Task.Objective
	answers := make([]fleetItemResult, len(subjects))
	var wg sync.WaitGroup
	for k, subject := range subjects {
		child := spec
		child.Task.Objective = fanoutItemObjective(template, subject)
		child.Task.Description = boundedInline(subject, 60)
		subID := fmt.Sprintf("%s/fleet-%d/item-%d", parentID, idx+1, k+1)
		dispatchArgs, _ := json.Marshal(map[string]any{
			"prompt":      child.Task.Objective,
			"description": child.Task.Description,
			"profile":     child.Worker.Profile,
		})
		sink.Emit(event.Event{
			Kind: event.ToolDispatch,
			Tool: event.Tool{
				ID: subID, ParentID: parentID, Name: "task",
				Args: string(dispatchArgs), ReadOnly: child.Grant.ReadOnly,
			},
		})
		wg.Go(func() {
			itemCtx := withCallContext(ctx, subID, subSinkFor(subID, sink), nil, false)
			out, err := f.taskTool.RunProfileSpec(itemCtx, child)
			answer, ref := splitSubagentRunResult(out)
			answers[k] = fleetItemResult{index: k, output: answer, ref: ref, err: err, status: fleetItemCompleted}
			if err != nil {
				answers[k].status = fleetItemFailed
				if ctx.Err() != nil {
					answers[k].status = fleetItemCancelled
				}
			}
		})
	}
	wg.Wait()

	failed := 0
	for _, a := range answers {
		if a.status != fleetItemCompleted {
			failed++
		}
	}
	res.output = formatFanoutAggregate(subjects, answers)
	// One broken subject does not discard the rest: the node reports what it
	// has and fails only when nothing came back, which is the case a dependent
	// cannot start from.
	res.status = fleetItemCompleted
	if failed == len(answers) {
		res.status = fleetItemFailed
		res.err = fmt.Errorf("every mapped subject failed")
	}
	return res
}

// formatFanoutAggregate renders the mapped answers under one shared budget, so
// a wide node cannot overrun what its dependents open with.
func formatFanoutAggregate(subjects []string, answers []fleetItemResult) string {
	items := make([]subagentAggregateItem, 0, len(answers))
	for k, a := range answers {
		item := subagentAggregateItem{header: "\u2500\u2500 " + boundedInline(subjects[k], 80) + " \u2500\u2500\n", ref: a.ref}
		switch a.status {
		case fleetItemCompleted:
			item.status = "status: completed\n"
			item.answer = strings.TrimSpace(a.output)
		default:
			item.status = "status: " + string(a.status) + "\n"
			if a.err != nil {
				item.detail = "[" + strings.ToUpper(string(a.status)) + "] " + boundedInline(a.err.Error(), 256) + "\n"
			}
		}
		items = append(items, item)
	}
	prefix := fmt.Sprintf("Mapped %d subject(s):\n", len(answers))
	return formatBoundedSubagentAggregate(prefix, items)
}
