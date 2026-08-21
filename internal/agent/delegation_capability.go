package agent

// The delegation namespace. These ids are the whole discovery surface for the
// dispatchers: the provider schema hides them, so the catalog lists exactly
// this set and use_capability resolves exactly this set.

func (*TaskTool) CapabilityAliases() []string { return []string{"task:subagent"} }

func (*ReadOnlyTaskTool) CapabilityAliases() []string { return []string{"task:read_only_subagent"} }

func (*ParallelTasksTool) CapabilityAliases() []string { return []string{"task:parallel"} }

func (*FleetTool) CapabilityAliases() []string { return []string{"task:fleet"} }
