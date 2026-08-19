package boot

import (
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/plugin"
	"reasonix/internal/pluginspec"
)

// mcpSpecPlan is what a build resolves about MCP servers before any process is
// touched: the specs per start tier, the names config has enabled, and the
// demotion notices earned by chronically slow eager servers.
type mcpSpecPlan struct {
	eager      []plugin.Spec
	background []plugin.Spec
	extra      []plugin.Spec
	enabled    map[string]bool
	demotions  []string
}

// resolveMCPSpecs partitions the enabled servers by start tier and normalizes
// the host-session extras. Legacy eager/background tiers no longer change
// process start timing; the partition survives so a chronically slow eager
// server can still be demoted with a notice the user can act on.
func resolveMCPSpecs(opts Options, cfg *config.Config, root string, specOptions pluginspec.Options) mcpSpecPlan {
	autoStart := cfg.EnabledPlugins(root, config.DefaultActivationStore())
	enabled := make(map[string]bool, len(autoStart))
	for _, e := range autoStart {
		if name := strings.TrimSpace(e.Name); name != "" {
			enabled[name] = true
		}
	}
	eagerEntries, bgEntries := partitionByTier(autoStart)
	extra := pluginspec.ApplyDefaultStartupTimeout(
		pluginspec.ApplyDefaultCallTimeout(
			pluginspec.ApplyKnownOverrides(opts.ExtraPlugins, root),
			specOptions.DefaultCallTimeout,
		),
		specOptions.DefaultStartupTimeout,
	)
	for i := range extra {
		if strings.TrimSpace(extra[i].WorkspaceRoot) == "" {
			extra[i].WorkspaceRoot = root
		}
		if extra[i].LaunchManager == nil {
			extra[i].LaunchManager = specOptions.LaunchManager
		}
		if strings.TrimSpace(extra[i].ConfigSource) == "" {
			extra[i].ConfigSource = "host_session"
		}
		if !extra[i].RequireLaunchApproval {
			// Session-scoped MCP specs arrive through an explicit host/user action
			// (for example ACP session/new), so they follow installed-server
			// authorization without another per-tool or per-session prompt.
			extra[i].Authorized = true
		}
		pluginspec.ApplyIsolation(&extra[i], root, specOptions)
	}

	// A chronically slow eager server (recent samples repeatedly hit the
	// blocking startup budget) drops to background for this session: the user
	// keeps eager intent without paying for it on a server that misbehaves.
	var demotions []string
	budget := plugin.DefaultStartupBudget()
	kept := eagerEntries[:0]
	for _, e := range eagerEntries {
		rec := plugin.Recommend(e.Name, budget, 0)
		if rec.Demote {
			demotions = append(demotions, rec.Reason)
			bgEntries = append(bgEntries, e)
			continue
		}
		kept = append(kept, e)
	}
	eagerEntries = kept

	eager := pluginspec.ForRootWithOptions(eagerEntries, root, specOptions)
	background := pluginspec.ForRootWithOptions(bgEntries, root, specOptions)
	eager = append(eager, extra...)

	if opts.Stderr != nil {
		for i := range eager {
			eager[i].Stderr = opts.Stderr
		}
		for i := range background {
			background[i].Stderr = opts.Stderr
		}
	}
	return mcpSpecPlan{eager: eager, background: background, extra: extra, enabled: enabled, demotions: demotions}
}
