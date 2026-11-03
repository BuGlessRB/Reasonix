package config

import (
	"slices"
	"strings"
)

// mcpIdentity returns the fixed part of a server's override identity: whether
// its scope is forced, and the disambiguators that keep two packages exposing
// the same short server name apart.
func mcpIdentity(entry PluginEntry) (forcedProject bool, source, owner string) {
	source = strings.TrimSpace(string(entry.Source))
	if entry.Source == MCPSourcePluginPackage {
		return false, source, source
	}
	// Project-scoped by nature: a global row would reach another project's
	// same-named declaration. Unknown provenance counts as repository-declared,
	// because Source survives only a merge that succeeded.
	if entry.Source.ProjectScoped() || source == "" || source == "workspace_config" || source == "project" || source == ".mcp.json" {
		return true, source, ""
	}
	return false, source, ""
}

// RepositoryDeclared reports an entry whose declaration arrives with the
// workspace, so an enable on it holds only for the declaration it was given for.
func RepositoryDeclared(entry PluginEntry) bool {
	repoDeclared, _, _ := mcpIdentity(entry)
	return repoDeclared
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

// ServerDecision reports what the store holds for entry in root, and whether an
// undecided one may start anyway. A repository-declared one may not, since a
// clone chose its command; and an enable on it holds only for the declaration
// it was given for, any other reading as ActivationChanged. Every other source
// is a file only the user writes.
func (s *ActivationStore) ServerDecision(entry PluginEntry, root string) (ActivationDecision, bool, error) {
	repoDeclared, _, _ := mcpIdentity(entry)
	row, found, err := s.decidingRow(ServerOverrideFor(entry, root, ActivationGlobal), root, repoDeclared)
	switch {
	case err != nil || !found:
		return ActivationUndecided, DeclaredDefaultOn(entry), err
	case repoDeclared && row.Enabled && !slices.Contains(confirmedDeclarations(row), ProjectDeclarationDigest(entry, root)):
		return ActivationChanged, DeclaredDefaultOn(entry), nil
	}
	return decisionOf(row.Enabled), DeclaredDefaultOn(entry), nil
}

// DeclaredDefaultOn is what a server does when the store cannot be read. Every
// error fallback must use it rather than auto_start, which would re-enable
// exactly the servers the gate is for.
func DeclaredDefaultOn(entry PluginEntry) bool {
	repoDeclared, _, _ := mcpIdentity(entry)
	return !repoDeclared && entry.ShouldAutoStart()
}

// IsEnabled resolves the product enable state for entry in root. An override
// wins, project layer first; without one a repository-declared server stays off
// and anything else follows auto_start.
func (s *ActivationStore) IsEnabled(entry PluginEntry, root string) (bool, error) {
	decision, defaultOn, err := s.ServerDecision(entry, root)
	if err != nil {
		return false, err
	}
	if decision != ActivationUndecided {
		return decision == ActivationEnabled, nil
	}
	return defaultOn, nil
}

// ServerChanged reports an enable the user gave for another declaration of
// entry, so a surface can say why the server is waiting again.
func (s *ActivationStore) ServerChanged(entry PluginEntry, root string) bool {
	decision, _, err := s.ServerDecision(entry, root)
	return err == nil && decision == ActivationChanged
}

// AwaitingDecision reports a repository-declared server nobody has answered for.
// It is off, but not because the user turned it off — a surface that cannot tell
// those apart shows a project's MCP as simply missing.
func (s *ActivationStore) AwaitingDecision(entry PluginEntry, root string) bool {
	decision, defaultOn, err := s.ServerDecision(entry, root)
	if err != nil {
		return !DeclaredDefaultOn(entry) // unread store: ask again rather than go quiet
	}
	return decision == ActivationChanged || decision == ActivationUndecided && !defaultOn
}

// SetServerEnabled records a durable decision for entry at scope.
// Enabling a repository-declared server records the declaration it was given
// for, as it reads now. Either way the write migrates the repository's old
// git-dir-keyed row, so only the current key resolves.
func (s *ActivationStore) SetServerEnabled(entry PluginEntry, root string, scope ActivationScope, enabled bool) error {
	override := ServerOverrideFor(entry, root, scope)
	override.Enabled = enabled
	var err error
	if repoDeclared, _, _ := mcpIdentity(entry); repoDeclared && enabled {
		err = s.confirmDeclaration(override, ProjectDeclarationDigest(entry, root))
	} else {
		err = s.SetOverride(override)
	}
	if err != nil {
		return err
	}
	return s.dropLegacyRepoRow(override, root)
}

// ClearServer removes entry's override at scope, restoring what it inherits. It
// also drops the old repository-keyed row so a stale one cannot resurface.
func (s *ActivationStore) ClearServer(entry PluginEntry, root string, scope ActivationScope) error {
	override := ServerOverrideFor(entry, root, scope)
	if err := s.ClearOverride(override); err != nil {
		return err
	}
	return s.dropLegacyRepoRow(override, root)
}

// dropLegacyRepoRow removes a project row filed under the repository's old
// git-dir key, which the caller's own write would otherwise leave behind. A
// non-repository root, or a key already current, needs no migration.
func (s *ActivationStore) dropLegacyRepoRow(override ActivationOverride, root string) error {
	if override.Scope != ActivationProject {
		return nil
	}
	legacy := legacyRepoKey(root)
	if legacy == "" || legacy == override.Key {
		return nil
	}
	override.Key = legacy
	return s.ClearOverride(override)
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

// maxConfirmedDeclarations bounds how many declarations one slot remembers. The
// slot is a row in a user file read at every launch, and a repository with eight
// projects declaring one name differently is already far past a real layout; past
// the bound the oldest confirmation is dropped, which only asks about it again.
const maxConfirmedDeclarations = 8

// confirmedDeclarations are the declaration digests the user answered yes for on
// this row: the list a newer binary writes plus the single Identity an older one
// did, so a file from before the list still reads.
func confirmedDeclarations(row ActivationOverride) []string {
	var out []string
	for _, d := range append(slices.Clone(row.Identities), row.Identity) {
		if d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// confirmDeclaration records an enable for digest on the slot override names. A
// project of the same repository that declares the same name differently shares
// the slot, so the enable adds to what the user already confirmed there instead
// of replacing it; only a declaration on that list may start. Disabling still
// replaces the whole row, which clears the list.
func (s *ActivationStore) confirmDeclaration(override ActivationOverride, digest string) error {
	if s == nil {
		return nil
	}
	override = normalizeOverride(override)
	if override.Name == "" {
		return nil
	}
	return s.update(func(file *ActivationFile) {
		confirmed := []string{}
		if prior, ok := findOverride(file.Overrides, overrideKey(override)); ok && prior.Enabled {
			confirmed = slices.DeleteFunc(confirmedDeclarations(prior), func(d string) bool { return d == digest })
		}
		confirmed = append(confirmed, digest)
		if len(confirmed) > maxConfirmedDeclarations {
			confirmed = confirmed[len(confirmed)-maxConfirmedDeclarations:]
		}
		override.Identity = digest
		override.Identities = confirmed
		file.Overrides = upsertOverride(file.Overrides, override)
	})
}
