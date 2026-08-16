package boot

import (
	"log/slog"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
)

// resolveTriageProvider builds the model that answers host classifications the
// static tables could not make: triage_model, then subagent_model, then the main
// model. The last step is what makes the escalation work with no configuration —
// a separate instance, so classifications never enter the stream the turn is
// mid-conversation on, and triage_model exists to point them at a cheaper tier.
func resolveTriageProvider(cfg *config.Config, mainRef string, proxySpec netclient.ProxySpec) provider.Provider {
	ref := firstNonEmpty(
		strings.TrimSpace(cfg.Agent.TriageModel),
		strings.TrimSpace(cfg.Agent.SubagentModel),
		strings.TrimSpace(mainRef),
	)
	if ref == "" {
		return nil
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok {
		return nil
	}
	prov, err := NewProviderWithProxy(entry, proxySpec)
	if err != nil {
		// Not fatal: the static verdict stands, which only ever refuses more.
		slog.Warn("triage provider construction failed — host keeps its static command classification", "model", ref, "err", err)
		return nil
	}
	return prov
}
