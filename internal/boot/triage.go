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
	prov, _, _ := resolveTriage(cfg, mainRef, proxySpec)
	return prov
}

// resolveTriage also answers which model was chosen and what it costs. A
// classification billed at the main tier's price is the one number that makes
// the escalation look expensive when the point of triage_model is that it is
// not, so the ref and pricing travel with the provider.
func resolveTriage(cfg *config.Config, mainRef string, proxySpec netclient.ProxySpec) (provider.Provider, string, *provider.Pricing) {
	ref := firstNonEmpty(
		strings.TrimSpace(cfg.Agent.TriageModel),
		strings.TrimSpace(cfg.Agent.SubagentModel),
		strings.TrimSpace(mainRef),
	)
	if ref == "" {
		return nil, "", nil
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok {
		return nil, "", nil
	}
	prov, err := NewProviderWithProxy(entry, proxySpec)
	if err != nil {
		// Not fatal: the static verdict stands, which only ever refuses more.
		slog.Warn("triage provider construction failed — host keeps its static command classification", "model", ref, "err", err)
		return nil, "", nil
	}
	return prov, ref, entry.Price
}
