// readonly_subagent.go — what a strict read-only child is given, and in what order.
package agent

import (
	"context"
	"fmt"

	"reasonix/internal/tool"
)

// readOnlySubRegistry builds a strict child's tools, proves it can investigate
// something, and only then gives it the way to report. The order is the
// invariant: complete_subtask is protocol, not capability, and attaching it
// first would let a child with no research tools pass the emptiness check by
// holding the tool it files findings with.
func (t *TaskTool) readOnlySubRegistry(spec *ProfileExecSpec, toolNames []string, childDepth int) (*tool.Registry, error) {
	sub := ReadOnlySubagentToolRegistryForDepthWithRuntime(t.parentReg, toolNames, childDepth, t.maxDepth(), t.capabilityRuntime)
	if sub.Len() == 0 && !spec.Grant.AllowNoTools {
		return nil, fmt.Errorf("no read-only tools available for this sub-agent")
	}
	AttachCompleteSubtaskTool(sub)
	return sub, nil
}

// prepareSubSession applies the host framing every delegated child gets: the
// workspace note, the closing contract, and the telemetry fields that say the
// contract was given. Shared so a runner cannot drift from the other on which
// of them a child is told.
func (t *TaskTool) prepareSubSession(ctx context.Context, prompt string, opts Options, modelRef, entrance string) (context.Context, string, Options) {
	opts.ModelRef = modelRef
	// The pristine task, before framing: delivery classification judges the
	// task and not the wrapper.
	opts.ClassifierTaskText = prompt
	opts.ExpectCompletionReport, opts.HandoffEntrance = true, entrance
	prompt = subagentImageNote(ctx) + t.withWorkspaceContext(upstreamNote(ctx)+prompt) + "\n\n" + completeSubtaskContract
	return WithUserImages(ctx, SubagentImageCandidates(ctx)), prompt, opts
}
