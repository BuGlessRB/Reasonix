package plugin

import "slices"

// ToolEnabled matches the server's raw tool name, before prefix stripping.
func (s Spec) ToolEnabled(rawName string) bool {
	return !slices.Contains(s.DisabledTools, rawName)
}

// EnabledCachedTools applies current policy even to a stale schema snapshot.
func (s Spec) EnabledCachedTools(tools []CachedTool) []CachedTool {
	if len(s.DisabledTools) == 0 {
		return tools
	}
	return slices.DeleteFunc(slices.Clone(tools), func(t CachedTool) bool { return !s.ToolEnabled(t.Name) })
}

func disabledToolNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	names = slices.Clone(names)
	slices.Sort(names)
	return slices.Compact(names)
}
