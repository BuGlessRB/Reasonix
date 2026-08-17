package control

import (
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/plugin"
)

// MCPScope is how far an installed server reaches. User follows the person
// across projects; Project follows the repository, which means it also reaches
// everyone who clones it. Local is the person's own server in one project only:
// installing is a global act either way, so it declares the server globally and
// then answers the "is it on" question per layer — off everywhere, on here.
type MCPScope string

const (
	MCPScopeUser    MCPScope = "user"
	MCPScopeProject MCPScope = "project"
	MCPScopeLocal   MCPScope = "local"
)

// InstallMCPServer connects a candidate server and persists it only once the
// handshake proves it works — a saved entry that never connects reads as
// installed while contributing no tools. A server that needs authentication is
// the exception: its config is kept, because completing OAuth and retrying is
// impossible once the entry is gone.
func (c *Controller) InstallMCPServer(e config.PluginEntry, scope MCPScope) (plugin.MCPInstallResult, error) {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return plugin.InstallResultForError("", fmt.Errorf("需要一个服务名")), nil
	}
	if existing, err := c.configuredMCPServer(e.Name); err == nil {
		return plugin.InstallResultForError(e.Name,
			fmt.Errorf("已经有一个叫 %q 的服务了（来自 %s）", e.Name, existing.Source)), nil
	}
	if scope == MCPScopeProject || scope == MCPScopeLocal {
		if strings.TrimSpace(c.workspaceRoot) == "" {
			return plugin.InstallResultForError(e.Name, fmt.Errorf("没有打开项目，装不了项目级的服务")), nil
		}
	}
	if scope == MCPScopeProject {
		e.Source = config.MCPSourceProjectConfig
	} else {
		e.Source = config.MCPSourceUserConfig
	}

	toolCount, connErr := c.connectMCPServer(e)
	if connErr != nil {
		result := plugin.InstallResultForError(e.Name, connErr)
		if result.State != "action_required" {
			// Nothing was persisted, so the name must not linger in the failure
			// list either — it would show up as a server the user never installed.
			if h := c.mcp.hostRef(); h != nil {
				h.ClearFailure(e.Name)
			}
			c.mcp.disconnect(e.Name)
			c.mcp.removeToolPrefix(e.Name)
			return result, nil
		}
		if err := c.persistMCPServer(e); err != nil {
			return plugin.MCPInstallResult{}, err
		}
		return result, nil
	}
	if err := c.persistMCPServer(e); err != nil {
		c.DisconnectMCPServer(e.Name)
		return plugin.MCPInstallResult{}, err
	}
	if err := c.confineMCPToThisProject(e, scope); err != nil {
		c.DisconnectMCPServer(e.Name)
		return plugin.MCPInstallResult{}, errors.Join(err, c.rollbackMCPServer(e.Name))
	}
	return plugin.ReadyInstallResult(e.Name, toolCount), nil
}

// confineMCPToThisProject turns a global declaration into a local one by
// answering the activation question per layer. Nothing else stores a
// per-project install, and nothing else needs to.
func (c *Controller) confineMCPToThisProject(e config.PluginEntry, scope MCPScope) error {
	if scope != MCPScopeLocal {
		return nil
	}
	store := config.DefaultActivationStore()
	if err := store.SetServerEnabled(e, c.workspaceRoot, config.ActivationGlobal, false); err != nil {
		return err
	}
	return store.SetServerEnabled(e, c.workspaceRoot, config.ActivationProject, true)
}

// persistMCPServer writes the declaration and its activation together, rolling
// the declaration back if activation fails: a server present in config but
// missing from the activation store resolves by auto_start, which is not what
// the user just chose.
func (c *Controller) persistMCPServer(e config.PluginEntry) error {
	var err error
	if e.Source == config.MCPSourceUserConfig {
		_, err = config.InstallUserPluginForRoot(c.workspaceRoot, e, true)
	} else {
		_, err = config.UpsertPluginInSourceForRoot(c.workspaceRoot, e)
	}
	if err != nil {
		return fmt.Errorf("保存配置: %w", err)
	}
	store := config.DefaultActivationStore()
	if !e.ShouldAutoStart() {
		if clearErr := store.ClearServer(e, c.workspaceRoot, config.ActivationGlobal); clearErr != nil {
			return errors.Join(clearErr, c.rollbackMCPServer(e.Name))
		}
		return nil
	}
	if setErr := store.SetServerEnabled(e, c.workspaceRoot, config.ActivationGlobal, true); setErr != nil {
		return errors.Join(setErr, c.rollbackMCPServer(e.Name))
	}
	return nil
}

func (c *Controller) rollbackMCPServer(name string) error {
	_, _, _, err := config.RemovePluginFromEffectiveSourceForRoot(c.workspaceRoot, name)
	return err
}

// MCPServerState is one configured server's declaration paired with the
// activation switch that decides whether this session may use it at all.
// LocalOverride marks a switch this project set for itself rather than
// inheriting, which the settings surface shows as a local exception.
type MCPServerState struct {
	Entry         config.PluginEntry
	Enabled       bool
	LocalOverride bool
}

// ConfiguredMCPServers lists every configured server with its resolved
// activation state, in config order. One config read and one activation read
// answer the whole list, which the per-name accessors cannot do.
func (c *Controller) ConfiguredMCPServers() []MCPServerState {
	cfg, err := config.LoadForRootReadOnly(c.workspaceRoot)
	if err != nil {
		return nil
	}
	store := config.DefaultActivationStore()
	overrides, _ := store.ProjectOverrides(c.workspaceRoot)
	local := make(map[string]bool, len(overrides))
	for _, row := range overrides {
		if row.Kind == config.CapabilityMCP {
			local[row.Name] = true
		}
	}
	out := make([]MCPServerState, 0, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		enabled, err := store.IsEnabled(p, c.workspaceRoot)
		if err != nil {
			enabled = p.ShouldAutoStart()
		}
		out = append(out, MCPServerState{Entry: p, Enabled: enabled, LocalOverride: local[p.Name]})
	}
	return out
}

// ReconnectMCPServer retries one configured server and re-registers its tools.
// The recorded failure is cleared first: a failed name is absent from Servers()
// until the record goes, so a successful retry would still read as broken.
func (c *Controller) ReconnectMCPServer(name string) (int, error) {
	entry, err := c.configuredMCPServer(strings.TrimSpace(name))
	if err != nil {
		return 0, err
	}
	if h := c.mcp.hostRef(); h != nil {
		h.ClearFailure(entry.Name)
	}
	c.mcp.disconnect(entry.Name)
	n, connErr := c.connectMCPServer(entry)
	if connErr != nil {
		// Without a record the server falls back to "configured but idle", which
		// reads as never attempted rather than attempted and still broken.
		if h := c.mcp.hostRef(); h != nil {
			h.RecordFailure(c.mcpSpec(entry), connErr)
		}
		return 0, connErr
	}
	return n, nil
}

// MCPServerEnabled resolves one configured server's durable activation state.
func (c *Controller) MCPServerEnabled(name string) (bool, error) {
	entry, err := c.configuredMCPServer(strings.TrimSpace(name))
	if err != nil {
		return false, err
	}
	return config.DefaultActivationStore().IsEnabled(entry, c.workspaceRoot)
}

// SetMCPServerEnabled persists the switch at scope and moves this session's
// tool registry with it. Enabling restores the cached tool surface without
// starting a process, so it stays cheap for a lazy server. A registry failure
// rolls the persisted state back: a switch that survives a restart but changed
// nothing now is a lie.
func (c *Controller) SetMCPServerEnabled(name string, scope config.ActivationScope, enabled bool) error {
	entry, err := c.configuredMCPServer(strings.TrimSpace(name))
	if err != nil {
		return err
	}
	store := config.DefaultActivationStore()
	prev, hadPrev, err := store.ServerOverride(entry, c.workspaceRoot, scope)
	if err != nil {
		return err
	}
	if err := store.SetServerEnabled(entry, c.workspaceRoot, scope, enabled); err != nil {
		return err
	}
	if !enabled {
		c.DisconnectMCPServer(entry.Name)
		return nil
	}
	if _, err := c.RegisterMCPServerOnDemand(entry); err != nil {
		if hadPrev {
			return errors.Join(err, store.SetOverride(prev))
		}
		return errors.Join(err, store.ClearServer(entry, c.workspaceRoot, scope))
	}
	return nil
}

// ClearMCPServerOverride drops this project's exception for name, returning it
// to whatever the global layer and its own declaration say.
func (c *Controller) ClearMCPServerOverride(name string, scope config.ActivationScope) error {
	entry, err := c.configuredMCPServer(strings.TrimSpace(name))
	if err != nil {
		return err
	}
	if err := config.DefaultActivationStore().ClearServer(entry, c.workspaceRoot, scope); err != nil {
		return err
	}
	enabled, err := config.DefaultActivationStore().IsEnabled(entry, c.workspaceRoot)
	if err != nil {
		return err
	}
	if !enabled {
		c.DisconnectMCPServer(entry.Name)
		return nil
	}
	_, err = c.RegisterMCPServerOnDemand(entry)
	return err
}
