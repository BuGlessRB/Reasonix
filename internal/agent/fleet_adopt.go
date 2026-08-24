package agent

import (
	"context"
	"fmt"
	"strings"
)

// adoptedItem is one fleet node that resolves to a prior child's answer instead
// of running. The reference is its identity, and the host verifies only what the
// structure can settle. Whether an answer is still current is not in the
// structure — a workspace moves on — so that judgement stays with the caller
// that listed the reference.
type adoptedItem struct {
	answer string
	ref    string
}

// adoptedItemExecutionKnob names the first field an adopted item set that only a
// run could use. Accepting one silently would be worse than refusing it: the
// caller would believe it took effect.
func adoptedItemExecutionKnob(item fleetTaskItem) string {
	for _, knob := range []struct {
		name string
		set  bool
	}{
		{"profile", strings.TrimSpace(item.Profile) != ""},
		{"write_paths", len(item.WritePaths) > 0},
		{"tools", len(item.Tools) > 0},
		{"max_steps", item.MaxSteps != 0},
		{"model", strings.TrimSpace(item.Model) != ""},
		{"effort", strings.TrimSpace(item.Effort) != ""},
		{"read_only", item.ReadOnly},
	} {
		if knob.set {
			return knob.name
		}
	}
	return ""
}

// validateFleetItemShape settles what an item is before anything is resolved:
// exactly one of prompt and adopt_ref, and nothing a run would need on a node
// that will not run.
func validateFleetItemShape(index int, item fleetTaskItem) error {
	prompt := strings.TrimSpace(item.Prompt)
	adopt := strings.TrimSpace(item.AdoptRef)
	switch {
	case prompt == "" && adopt == "":
		return fmt.Errorf("task %d: prompt is required, or adopt_ref to stand in for running it", index+1)
	case prompt != "" && adopt != "":
		return fmt.Errorf("task %d: prompt and adopt_ref are mutually exclusive; an adopted item is not run, so it has no prompt", index+1)
	case adopt == "":
		return nil
	}
	if knob := adoptedItemExecutionKnob(item); knob != "" {
		return fmt.Errorf("task %d: %s is not valid on an adopted item, which runs nothing; drop it or drop adopt_ref", index+1, knob)
	}
	return nil
}

// validateFanoutItemShape settles what a mapped item may say about itself.
// max_items without for_each is the case worth refusing loudest: it reads as a
// ceiling and would bound nothing at all.
func validateFanoutItemShape(index int, item fleetTaskItem) error {
	forEach := strings.TrimSpace(item.ForEach)
	switch {
	case forEach != "" && strings.TrimSpace(item.AdoptRef) != "":
		return fmt.Errorf("task %d: for_each and adopt_ref are mutually exclusive; an adopted item runs nothing to map", index+1)
	case forEach == "" && item.MaxItems != 0:
		return fmt.Errorf("task %d: max_items bounds a for_each task's width and bounds nothing without one", index+1)
	case forEach != "" && !item.ReadOnly:
		return fmt.Errorf("task %d: a for_each task must set read_only; its width is not known at preflight, so its writers could not be checked against each other", index+1)
	}
	return nil
}

// resolveAdoptions verifies every adopt_ref before anything starts, matching the
// rest of the preflight: a fleet that discovers halfway through that it cannot
// finish has already spent tokens. ReadFinalAnswer is the single gate, so what
// may be adopted cannot drift from what read_subagent_result may read.
func (f *FleetTool) resolveAdoptions(ctx context.Context, items []fleetTaskItem) (map[int]adoptedItem, error) {
	var adopted map[int]adoptedItem
	runnable := 0
	for i, item := range items {
		ref := strings.TrimSpace(item.AdoptRef)
		if ref == "" {
			runnable++
			continue
		}
		answer, err := f.adoptedAnswer(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		if adopted == nil {
			adopted = make(map[int]adoptedItem, len(items))
		}
		adopted[i] = adoptedItem{answer: answer, ref: ref}
	}
	if len(adopted) > 0 && runnable == 0 {
		return nil, fmt.Errorf("every task is adopted, so the fleet would run nothing; read those results directly instead")
	}
	return adopted, nil
}

func (f *FleetTool) adoptedAnswer(ctx context.Context, ref string) (string, error) {
	if f.taskTool.transcripts == nil {
		return "", fmt.Errorf("adopt_ref needs persisted sub-agent transcripts, which this session does not keep")
	}
	parentSession := ParentSession(ctx)
	if parentSession == "" {
		return "", fmt.Errorf("adopt_ref needs a persisted parent session")
	}
	answer, _, err := f.taskTool.transcripts.ReadFinalAnswer(ref, parentSession, f.taskTool.workspaceRoot)
	if err != nil {
		return "", err
	}
	return answer, nil
}
