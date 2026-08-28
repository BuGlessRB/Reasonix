package boot

import (
	"reasonix/internal/ablation"
	"reasonix/internal/agentpreset"
	"reasonix/internal/tool"
	"reasonix/internal/tool/builtin"
)

// Canonical Agent role-setting identifiers re-exported for frontends that
// already import boot. Prefer agentpreset directly in new code.
const (
	AgentPresetBalanced = string(agentpreset.Balanced)
	AgentPresetDelivery = string(agentpreset.Delivery)
)

// Deprecated TokenMode constants — one compatibility version. Prefer
// AgentPreset* and NormalizeAgentPreset.
const (
	TokenModeFull     = "full"
	TokenModeEconomy  = "economy"
	TokenModeDelivery = "delivery"
)

// NormalizeAgentPreset maps free-form and legacy values to a canonical
// agent role setting. Empty/unknown → balanced.
func NormalizeAgentPreset(raw string) string {
	return string(agentpreset.Normalize(raw))
}

// NormalizeTokenMode is the deprecated alias that returns legacy tokenMode
// names (economy/full/delivery) for dual-write and older clients.
func NormalizeTokenMode(mode string) string {
	return agentpreset.LegacyTokenMode(agentpreset.Normalize(mode))
}

// AgentPresetFromTokenMode maps a legacy tokenMode onto a role setting.
func AgentPresetFromTokenMode(mode string) string {
	return string(agentpreset.FromLegacyTokenMode(mode))
}

// TokenModeFromAgentPreset maps a role setting onto the dual-write tokenMode.
func TokenModeFromAgentPreset(preset string) string {
	return agentpreset.LegacyTokenMode(agentpreset.Normalize(preset))
}

// CoreProviderToolNames is the stable top-level tool surface shared by every
// Agent role setting under identical configuration. Host-control tools
// (ask, update_goal, todo_write, complete_step) are appended when enabled.
func CoreProviderToolNames() []string {
	return []string{
		"bash",
		"bash_output",
		"kill_shell",
		"wait",
		"read_file",
		"edit_file",
		"write_file",
		"compress",
		"recall",
		"context_budget",
		"use_capability",
	}
}

// HostControlToolNames are collaboration/contract tools that may appear in the
// provider schema independently of the Agent role setting.
func HostControlToolNames() []string {
	return []string{
		"ask",
		"update_goal",
		"todo_write",
		"complete_step",
	}
}

// GoalOnlyToolNames are host-control tools whose contract exists only inside a
// Goal turn. An assembly that can never arm one drops them from the schema
// rather than shipping a definition the model can only fail to call.
func GoalOnlyToolNames() []string {
	return []string{"update_goal"}
}

// EvidenceToolNames are the tools whose whole purpose is the evidence
// contract, which the evidence arm drops from the schema. todo_write is not
// one: a task list is not an evidence claim, and an arm removing both would
// measure two things at once.
func EvidenceToolNames() []string {
	return []string{"complete_step"}
}

// UnifiedProviderToolNames returns the provider-visible allowlist for a boot
// with host-control tools enabled.
func UnifiedProviderToolNames() []string {
	core := CoreProviderToolNames()
	host := HostControlToolNames()
	out := make([]string, 0, len(core)+len(host))
	out = append(out, core...)
	out = append(out, host...)
	return out
}

// applyUnifiedProviderToolSurface restricts Schemas/ContractEntries to the
// shared core + host-control tools. use_capability can still Get every
// registered tool, including those hidden from the provider schema.
func applyUnifiedProviderToolSurface(reg *tool.Registry, goalTurnsUnreachable bool, arm ablation.Set) {
	if reg == nil {
		return
	}
	unreachable := map[string]bool{}
	if goalTurnsUnreachable {
		for _, name := range GoalOnlyToolNames() {
			unreachable[name] = true
		}
	}
	// The arm has to reach the schema, not only the gate: left visible, both
	// runs pay the same prompt and the same evidence arguments, and the
	// comparison answers for the gate when the question was the contract.
	if arm.Off(ablation.Evidence) {
		for _, name := range EvidenceToolNames() {
			unreachable[name] = true
		}
	}
	// recall keeps its read half, so the arm swaps the tool rather than hiding
	// it: an address the fold index still carries stays readable either way.
	if arm.Off(ablation.RecallSearch) {
		reg.Add(builtin.RecallWithoutSearch())
	}
	allow := make([]string, 0, 16)
	for _, name := range UnifiedProviderToolNames() {
		if unreachable[name] {
			continue
		}
		if _, ok := reg.Get(name); ok {
			allow = append(allow, name)
		}
	}
	// Always keep use_capability if somehow only that remains.
	if len(allow) == 0 {
		if _, ok := reg.Get("use_capability"); ok {
			allow = []string{"use_capability"}
		}
	}
	reg.SetProviderVisibleTools(allow)
}

// ApplyUnifiedProviderToolSurface restricts a registry to what a provider is
// shown, at this arm. Exported so a harness reproduces the real surface
// instead of a second copy of this rule.
func ApplyUnifiedProviderToolSurface(reg *tool.Registry, goalTurnsUnreachable bool, arm ablation.Set) {
	applyUnifiedProviderToolSurface(reg, goalTurnsUnreachable, arm)
}
