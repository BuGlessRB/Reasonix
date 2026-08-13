package control

import (
	"errors"
	"strings"

	"reasonix/internal/config"
)

// MCPServerState is one configured server's declaration paired with the
// activation switch that decides whether this session may use it at all.
type MCPServerState struct {
	Entry   config.PluginEntry
	Enabled bool
}

// ConfiguredMCPServers lists every configured server with its resolved
// activation state, in config order. One config read and one activation read
// answer the whole list, which the per-name accessors cannot do.
func (c *Controller) ConfiguredMCPServers() []MCPServerState {
	cfg, err := config.LoadForRootReadOnly(c.workspaceRoot)
	if err != nil {
		return nil
	}
	store := config.DefaultMCPActivationStore()
	out := make([]MCPServerState, 0, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		enabled, err := store.IsEnabled(p, c.workspaceRoot)
		if err != nil {
			enabled = p.ShouldAutoStart()
		}
		out = append(out, MCPServerState{Entry: p, Enabled: enabled})
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
	return config.DefaultMCPActivationStore().IsEnabled(entry, c.workspaceRoot)
}

// SetMCPServerEnabled persists the activation switch and moves this session's
// tool registry with it. Enabling restores the cached tool surface without
// starting a process — the first real call does that — so the switch stays cheap
// for a lazy server. A registry failure rolls the persisted state back, because
// a switch that survives a restart but changed nothing now is a lie.
func (c *Controller) SetMCPServerEnabled(name string, enabled bool) error {
	entry, err := c.configuredMCPServer(strings.TrimSpace(name))
	if err != nil {
		return err
	}
	store := config.DefaultMCPActivationStore()
	scope, workspaceFP, source, owner := config.ActivationIdentity(entry, c.workspaceRoot)
	prev, hadPrev, err := store.Lookup(scope, workspaceFP, source, owner, entry.Name)
	if err != nil {
		return err
	}
	if err := store.SetServerEnabled(entry, c.workspaceRoot, enabled); err != nil {
		return err
	}
	if !enabled {
		c.DisconnectMCPServer(entry.Name)
		return nil
	}
	if _, err := c.RegisterMCPServerOnDemand(entry); err != nil {
		if hadPrev {
			return errors.Join(err, store.SetServerEnabled(entry, c.workspaceRoot, prev))
		}
		return errors.Join(err, store.ClearServer(entry, c.workspaceRoot))
	}
	return nil
}
