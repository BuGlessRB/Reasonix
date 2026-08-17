package config

import "strings"

// mcpIdentity returns the fixed part of a server's override identity: whether
// its scope is forced, and the disambiguators that keep two packages exposing
// the same short server name apart.
func mcpIdentity(entry PluginEntry) (forcedProject bool, source, owner string) {
	source = strings.TrimSpace(string(entry.Source))
	if entry.Source == MCPSourcePluginPackage {
		return false, source, source
	}
	// A repository-declared server is project-scoped by nature: a global row for
	// it would reach another project's same-named declaration.
	if entry.Source.ProjectScoped() || source == "workspace_config" || source == "project" || source == ".mcp.json" {
		return true, source, ""
	}
	return false, source, ""
}

// ServerOverrideFor builds the override identity for entry at scope. Callers
// name the scope they mean; a repository-declared server is pinned to the
// project regardless of what was asked for.
func ServerOverrideFor(entry PluginEntry, root string, scope ActivationScope) ActivationOverride {
	forcedProject, source, owner := mcpIdentity(entry)
	if forcedProject {
		scope = ActivationProject
	}
	override := ActivationOverride{
		Kind:   CapabilityMCP,
		Scope:  scope,
		Source: source,
		Owner:  owner,
		Name:   strings.TrimSpace(entry.Name),
	}
	return placeProjectRow(override, root)
}

// IsEnabled resolves the product enable state for one plugin entry in root. An
// override wins, project layer first; otherwise auto_start=false maps to
// disabled and true/nil map to enabled.
func (s *ActivationStore) IsEnabled(entry PluginEntry, root string) (bool, error) {
	return s.Resolve(ServerOverrideFor(entry, root, ActivationGlobal), root, entry.ShouldAutoStart())
}

// SetServerEnabled records a durable decision for entry at scope.
func (s *ActivationStore) SetServerEnabled(entry PluginEntry, root string, scope ActivationScope, enabled bool) error {
	override := ServerOverrideFor(entry, root, scope)
	override.Enabled = enabled
	return s.SetOverride(override)
}

// ClearServer removes entry's override at scope, restoring what it inherits.
func (s *ActivationStore) ClearServer(entry PluginEntry, root string, scope ActivationScope) error {
	return s.ClearOverride(ServerOverrideFor(entry, root, scope))
}

// ClearServerEverywhere drops entry's rows at both layers. Uninstall uses it:
// leaving a row behind would silently govern a same-named server installed
// later. Only this workspace's project row is reachable — another project's row
// is keyed by an identity this process cannot enumerate.
func (s *ActivationStore) ClearServerEverywhere(entry PluginEntry, root string) error {
	if err := s.ClearServer(entry, root, ActivationGlobal); err != nil {
		return err
	}
	return s.ClearServer(entry, root, ActivationProject)
}

// ServerOverride returns the stored row for entry at scope, so a caller that
// must undo its own write can put back exactly what was there.
func (s *ActivationStore) ServerOverride(entry PluginEntry, root string, scope ActivationScope) (ActivationOverride, bool, error) {
	if s == nil {
		return ActivationOverride{}, false, nil
	}
	file, err := s.Load()
	if err != nil {
		return ActivationOverride{}, false, err
	}
	row, ok := findOverride(file.Overrides, overrideKey(ServerOverrideFor(entry, root, scope)))
	return row, ok, nil
}
