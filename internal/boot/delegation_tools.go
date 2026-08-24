package boot

import (
	"reasonix/internal/agent"
	"reasonix/internal/tool"
)

// addDelegationTools registers every tool that spawns or reads a sub-agent.
// The registry exports schemas in stable name order, and this surface is
// deliberately static: profile names and result refs never enter
// provider-visible schemas, and neither reader changes between turns.
func addDelegationTools(reg *tool.Registry, taskTool *agent.TaskTool) {
	reg.Add(taskTool)
	reg.Add(agent.NewParallelTasksTool(taskTool, reg))
	reg.Add(agent.NewFleetTool(taskTool))
	reg.Add(agent.NewSubagentResultTool(taskTool))
	reg.Add(agent.NewSubagentListTool(taskTool))
	reg.Add(agent.NewReadOnlyTaskTool(taskTool))
}
