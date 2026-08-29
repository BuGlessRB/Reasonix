// readonly_subagent.go — what a strict read-only child is given, and in what order.
package agent

import (
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
