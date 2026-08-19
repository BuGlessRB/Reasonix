package boot

import (
	"fmt"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
	"reasonix/internal/skill"
)

// subagentConfig is what a build resolves about sub-agents before any of them
// runs: which provider a named model or profile lands on, the depth and
// concurrency ceilings, and the profile lookups the task tool consults.
type subagentConfig struct {
	resolveProvider func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error)
	identity        func(modelRef, effort string) (string, string)
	profileLookup   func(name string) (agent.ProfileDefinition, bool)
	profileModel    func(profile string) string
	profileEffort   func(profile string) string
	scheduler       *agent.SubagentScheduler
	taskModel       string
	taskEffort      string
	maxDepth        int
}

func newSubagentConfig(opts Options, cfg *config.Config, entry *config.ProviderEntry, modelName string,
	resolver provider.Resolver, proxy netclient.ProxySpec, skills *skill.Store) subagentConfig {
	maxConcurrency, maxWriters := agent.NormalizeConcurrencyLimits(
		cfg.Agent.MaxSubagentConcurrency, cfg.Agent.MaxParallelWriters,
	)
	return subagentConfig{
		resolveProvider: func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error) {
			me := *entry
			selectedRef := modelRefFromEntry(entry)
			if strings.TrimSpace(modelRef) != "" {
				if resolved, ok := cfg.ResolveModel(modelRef); ok {
					me = *resolved
					selectedRef = modelRefFromEntry(resolved)
				} else if resolver != nil {
					me = *syntheticEntryFromResolver(resolver, modelRef)
					selectedRef = modelRef
				} else {
					return nil, nil, 0, fmt.Errorf("unknown model %q", modelRef)
				}
			}
			var effortOverride *string
			if strings.TrimSpace(effort) != "" {
				normalized, err := config.NormalizeEffort(&me, effort)
				if err != nil {
					if resolver == nil {
						return nil, nil, 0, err
					}
					normalized = effort
				}
				me.Effort = normalized
				effortOverride = &normalized
				if me.Kind == "anthropic" && strings.TrimSpace(me.Effort) != "" && strings.TrimSpace(me.Thinking) == "" {
					me.Thinking = "adaptive"
				}
			}
			p, err := resolveProvider(resolver, cfg, proxy, provider.Selection{Ref: selectedRef, Effort: effortOverride})
			if err != nil {
				return nil, nil, 0, err
			}
			return p, me.Price, me.ContextWindow, nil
		},
		identity: func(modelRef, effort string) (string, string) {
			return subagentEffectiveIdentity(cfg, opts.ProviderResolver, modelName, entry, modelRef, effort)
		},
		profileLookup: func(name string) (agent.ProfileDefinition, bool) {
			sk, ok := skills.Read(name)
			if !ok || sk.RunAs != skill.RunSubagent {
				return agent.ProfileDefinition{}, false
			}
			return agent.ProfileFromSkill(skills.Prepare(sk)), true
		},
		profileModel:  func(profile string) string { return firstConfigured(cfg.Agent.SubagentModels, profile) },
		profileEffort: func(profile string) string { return firstConfigured(cfg.Agent.SubagentEfforts, profile) },
		scheduler:     agent.NewSubagentScheduler(maxConcurrency, maxWriters),
		taskModel:     firstNonEmpty(cfg.Agent.SubagentModels["task"], cfg.Agent.SubagentModel),
		taskEffort:    firstNonEmpty(cfg.Agent.SubagentEfforts["task"], cfg.Agent.SubagentEffort),
		maxDepth:      agent.NormalizeMaxSubagentDepth(cfg.Agent.MaxSubagentDepth),
	}
}

// firstConfigured returns the first non-empty value among the keys a profile
// answers to, so an alias inherits the setting written under its canonical name.
func firstConfigured(values map[string]string, profile string) string {
	for _, key := range SubagentModelKeys(profile) {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	return ""
}
