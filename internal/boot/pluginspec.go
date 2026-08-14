package boot

import (
	"os"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/plugin"
	"reasonix/internal/sandbox"
	"reasonix/internal/skill"
	"reasonix/internal/tool"
)

// partitionByTier splits configured plugin entries into eager (block boot until
// ready) and background (placeholder + start spawn now). Entries with an empty,
// legacy lazy, or unrecognised tier land in background.
func partitionByTier(entries []config.PluginEntry) (eager, bg []config.PluginEntry) {
	for _, e := range entries {
		switch e.ResolvedTier() {
		case "eager":
			eager = append(eager, e)
		default:
			bg = append(bg, e)
		}
	}
	return eager, bg
}

// PluginSpecs maps configured plugin entries to plugin.Spec, expanding ${VAR}
// references. Exported so custom assemblers can connect the config's plugins
// alongside their own (e.g. ACP's per-session MCP servers).
func PluginSpecs(entries []config.PluginEntry) []plugin.Spec {
	return PluginSpecsForRoot(entries, "")
}

// PluginSpecsForRoot maps configured plugin entries to plugin.Spec and applies
// workspace-aware compatibility overrides for known cwd-sensitive servers.
func PluginSpecsForRoot(entries []config.PluginEntry, workspaceRoot string) []plugin.Spec {
	return PluginSpecsForRootWithOptions(entries, workspaceRoot, PluginSpecOptions{})
}

// PluginSpecsForRootWithOptions maps configured plugin entries to plugin.Spec
// and injects runtime policy such as the global MCP call timeout.
func PluginSpecsForRootWithOptions(entries []config.PluginEntry, workspaceRoot string, opts PluginSpecOptions) []plugin.Spec {
	specs := make([]plugin.Spec, len(entries))
	for i, e := range entries {
		specs[i] = pluginSpecFromEntryWithOptions(e, workspaceRoot, opts)
	}
	return specs
}

func pluginSpecFromEntryWithOptions(e config.PluginEntry, workspaceRoot string, opts PluginSpecOptions) plugin.Spec {
	e = e.ExpandedPlugin() // resolve ${VAR} / ${VAR:-default} from the environment
	configSource := strings.TrimSpace(string(e.Source))
	if configSource == "" {
		configSource = opts.ConfigSource
	}
	spec := plugin.ApplyKnownOverrides(plugin.Spec{
		Name:                  e.Name,
		Package:               strings.TrimSpace(opts.PackageOwners[e.Name]),
		Type:                  e.Type,
		Command:               e.Command,
		Args:                  e.Args,
		Env:                   e.Env,
		URL:                   e.URL,
		Headers:               e.Headers,
		DefaultStartupTimeout: opts.DefaultStartupTimeout,
		StartupTimeout:        secondsDuration(e.StartupTimeoutSeconds),
		DefaultCallTimeout:    opts.DefaultCallTimeout,
		CallTimeout:           secondsDuration(e.CallTimeoutSeconds),
		ToolTimeouts:          toolTimeoutDurations(e.ToolTimeoutSeconds),
		WorkspaceRoot:         strings.TrimSpace(workspaceRoot),
		LaunchManager:         opts.LaunchManager,
		ConfigSource:          configSource,
		Authorized:            e.Source.UserAuthorized(),
		OAuthHTTPClient:       opts.OAuthHTTPClient,
	}, workspaceRoot)
	if e.Source.ProjectScoped() && strings.TrimSpace(spec.Dir) == "" {
		spec.Dir = workspaceRoot
	}
	applyMCPIsolation(&spec, workspaceRoot, opts)
	return spec
}

func pluginPackageOwners(cfg *config.Config) map[string]string {
	out := map[string]string{}
	if cfg == nil {
		return out
	}
	for _, configured := range cfg.Plugins {
		if owner, ok := cfg.PluginPackageOwner(configured.Name); ok {
			out[configured.Name] = owner
		}
	}
	return out
}

func skillMCPBindings(sk skill.Skill, reg *tool.Registry, specs []plugin.Spec, cachedTools map[string][]plugin.CachedTool, cacheKeyOK map[string]bool) []tool.MCPBinding {
	var out []tool.MCPBinding
	liveServers := map[string]bool{}
	if reg != nil {
		bindings := reg.MCPBindings()
		out = make([]tool.MCPBinding, 0, len(bindings))
		for _, binding := range bindings {
			liveServers[binding.Server] = true
			if binding.Package == sk.Plugin {
				out = append(out, binding)
			}
		}
	}
	// A valid cached schema also supplies stable bindings for an on-demand
	// package server before it is connected. The skill can then route through
	// use_capability without inventing Reasonix's canonical name.
	for _, spec := range specs {
		if spec.Package != sk.Plugin || liveServers[spec.Name] || !cacheKeyOK[spec.Name] {
			continue
		}
		for _, cached := range cachedTools[spec.Name] {
			visible := cached.Name
			if spec.StripRawPrefix != "" {
				visible = strings.TrimPrefix(visible, spec.StripRawPrefix)
			}
			out = append(out, tool.MCPBinding{
				Package:      spec.Package,
				Server:       spec.Name,
				RawName:      cached.Name,
				VisibleName:  visible,
				CallableName: plugin.ModelToolName(spec.Name, visible),
				CapabilityID: "mcp-tool:" + spec.Name + "/" + cached.Name,
			})
		}
	}
	return out
}

func applyMCPIsolation(spec *plugin.Spec, workspaceRoot string, opts PluginSpecOptions) {
	if spec == nil {
		return
	}
	// Authorized user MCP defaults to trusted host process mode. Confined mode
	// is opt-in for internal managed deployments/tests and is never selected by
	// ordinary install paths.
	if spec.ProcessMode == "" {
		spec.ProcessMode = plugin.MCPProcessHost
	}
	if strings.TrimSpace(opts.StateHome) == "" {
		return
	}
	stateDir := plugin.MCPStateDir(opts.StateHome, workspaceRoot, spec.Name)
	spec.StateDir = stateDir
	if spec.ResolvedProcessMode() != plugin.MCPProcessConfined {
		// Host mode still gets a private state/cache/temp tree; only the OS
		// command sandbox is omitted so local app integrations keep working.
		return
	}
	writerRoots := appendUniquePaths([]string{stateDir}, opts.WriterRoots...)
	readerRoots := []string{workspaceRoot}
	if home, err := os.UserHomeDir(); err == nil {
		readerRoots = appendUniquePaths(readerRoots, home)
	}
	spec.Sandbox = sandbox.Spec{
		Mode: "enforce", WriteRoots: writerRoots,
		ReadRoots:              readerRoots,
		AppContainerWriteRoots: append([]string(nil), writerRoots...),
		ForbidReadRoots:        append([]string(nil), opts.ForbidReadRoots...),
		Network:                opts.Network, MinimalWrites: true,
	}
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func toolTimeoutDurations(seconds map[string]int) map[string]time.Duration {
	if len(seconds) == 0 {
		return nil
	}
	out := make(map[string]time.Duration, len(seconds))
	for name, sec := range seconds {
		name = strings.TrimSpace(name)
		if name == "" || sec <= 0 {
			continue
		}
		out[name] = time.Duration(sec) * time.Second
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyKnownPluginOverrides(specs []plugin.Spec, workspaceRoot string) []plugin.Spec {
	out := make([]plugin.Spec, len(specs))
	for i, spec := range specs {
		out[i] = plugin.ApplyKnownOverrides(spec, workspaceRoot)
	}
	return out
}

func applyDefaultMCPCallTimeout(specs []plugin.Spec, timeout time.Duration) []plugin.Spec {
	if len(specs) == 0 || timeout <= 0 {
		return specs
	}
	out := make([]plugin.Spec, len(specs))
	for i, spec := range specs {
		out[i] = spec
		if out[i].DefaultCallTimeout <= 0 {
			out[i].DefaultCallTimeout = timeout
		}
	}
	return out
}

func applyDefaultMCPStartupTimeout(specs []plugin.Spec, timeout time.Duration) []plugin.Spec {
	if len(specs) == 0 || timeout <= 0 {
		return specs
	}
	out := make([]plugin.Spec, len(specs))
	for i, spec := range specs {
		out[i] = spec
		if out[i].DefaultStartupTimeout <= 0 {
			out[i].DefaultStartupTimeout = timeout
		}
	}
	return out
}
