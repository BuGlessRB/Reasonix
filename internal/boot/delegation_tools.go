package boot

import (
	"reasonix/internal/agent"
	"reasonix/internal/tool"
)

// fleetFanoutEnabled is whether fleet exposes for_each. Off, with no setting to
// turn it on: measured against the same call written as one task, mapping spent
// about twice the tokens across two fixtures and twenty graded runs and never
// solved anything the single task did not (benchmarks/fleet-fanout). Enabling it
// is a decision for here, once the case one child cannot hold is measured.
const fleetFanoutEnabled = false

// addDelegationTools registers every tool that spawns or reads a sub-agent.
// The registry exports schemas in stable name order, and this surface is
// deliberately static: profile names and result refs never enter
// provider-visible schemas, and neither reader changes between turns.
func addDelegationTools(reg *tool.Registry, taskTool *agent.TaskTool, fanout bool) {
	reg.Add(taskTool)
	reg.Add(agent.NewParallelTasksTool(taskTool, reg))
	reg.Add(agent.NewFleetTool(taskTool).WithFanout(fanout))
	reg.Add(agent.NewSubagentResultTool(taskTool))
	reg.Add(agent.NewSubagentListTool(taskTool))
	reg.Add(agent.NewReadOnlyTaskTool(taskTool))
}
