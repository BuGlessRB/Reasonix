package config

import "strings"

// The vendor added an image-taking model to an endpoint we already ship. A
// stored model list cannot learn that by itself, so an installed user would see
// nothing change on upgrade and would have to know the name to type it in.

// DeepSeekVisionModel is the official DeepSeek model that reads images. Named
// here because three decisions need the same string: what to backfill, what a
// fresh install ships, and which model the image notice may point at.
const DeepSeekVisionModel = "deepseek-v4-flash-vision-exp"

// legacyDeepSeekV4Models is the list shipped before that model existed. A entry
// still carrying exactly it has not been curated, which is the only case this
// may touch — the same test migrateKimiK3VisionModels makes.
var legacyDeepSeekV4Models = []string{"deepseek-v4-flash", "deepseek-v4-pro"}

// deepSeekDefaultProviders is what a fresh install ships. Anthropic-compatible
// Messages, so provider-executed web search is on by default; existing explicit
// entries merge on top and keep the protocol they were configured with.
func deepSeekDefaultProviders() []ProviderEntry {
	return []ProviderEntry{
		{
			Name: "deepseek-flash", Kind: "anthropic", BaseURL: deepSeekAnthropicBaseURL,
			Model: "deepseek-v4-flash", APIKeyEnv: "DEEPSEEK_API_KEY",
			BalanceURL: "https://api.deepseek.com/user/balance", Thinking: "enabled",
			WebSearch: new(true), SupportedEfforts: []string{"disabled", "low", "high", "max"}, DefaultEffort: "low",
			ContextWindow: 1_000_000, Price: deepSeekV4FlashPriceUSD(),
			BillingCurrency: "USD", BillingMode: "payg",
		},
		{
			Name: "deepseek-pro", Kind: "anthropic", BaseURL: deepSeekAnthropicBaseURL,
			Model: "deepseek-v4-pro", APIKeyEnv: "DEEPSEEK_API_KEY",
			BalanceURL: "https://api.deepseek.com/user/balance", Thinking: "enabled",
			WebSearch: new(true), SupportedEfforts: []string{"disabled", "high", "max"}, DefaultEffort: "high",
			ContextWindow: 1_000_000, Price: deepSeekV4ProPriceUSD(),
			BillingCurrency: "USD", BillingMode: "payg",
		},
		deepSeekVisionProvider(),
	}
}

// deepSeekVisionProvider is the connection a fresh install ships for it. Ticked,
// because this is the one model on the host that reads images and an unticked
// one drops them; no web-search claim, because nothing has established it.
func deepSeekVisionProvider() ProviderEntry {
	return ProviderEntry{
		Name: "deepseek-vision", Kind: "anthropic", BaseURL: deepSeekAnthropicBaseURL,
		Model: DeepSeekVisionModel, APIKeyEnv: "DEEPSEEK_API_KEY",
		VisionModels: []string{DeepSeekVisionModel},
		BalanceURL:   "https://api.deepseek.com/user/balance", Thinking: "enabled",
		SupportedEfforts: []string{"disabled", "low", "high", "max"}, DefaultEffort: "high",
		ContextWindow: 1_000_000, Price: deepSeekV4FlashPriceUSD(),
		BillingCurrency: "USD", BillingMode: "payg",
	}
}

// normalizeOfficialDeepSeekModels brings a loaded config up to the catalog the
// vendor serves today. It reports whether it changed anything, so a load for
// edit persists that and a plain load only holds it in memory.
func normalizeOfficialDeepSeekModels(c *Config) bool {
	if c == nil {
		return false
	}
	changed := false
	for i := range c.Providers {
		p := &c.Providers[i]
		if officialProviderHost(p.BaseURL) != "api.deepseek.com" {
			continue
		}
		changed = applyDeepSeekVisionCatalog(p) || changed
		switch strings.TrimSpace(p.Name) {
		case "deepseek":
			required := []string{"deepseek-v4-flash", "deepseek-v4-pro"}
			if strings.EqualFold(strings.TrimSpace(p.Kind), "responses") {
				required = required[:1]
			}
			ensureProviderModels(p, required, "deepseek-v4-flash")
		case "deepseek-flash":
			ensureProviderModels(p, []string{"deepseek-v4-flash"}, "deepseek-v4-flash")
		case "deepseek-pro":
			ensureProviderModels(p, []string{"deepseek-v4-pro"}, "deepseek-v4-pro")
		}
		backfillDeepSeekAnthropicCapabilities(p)
	}
	return changed
}

// applyDeepSeekVisionCatalog offers the image-taking model to an official
// DeepSeek entry that still carries the shipped list. It reports whether it
// changed anything, so a load-for-edit persists it and a plain load does not.
func applyDeepSeekVisionCatalog(p *ProviderEntry) bool {
	if p == nil || officialProviderHost(p.BaseURL) != "api.deepseek.com" {
		return false
	}
	// Responses serves one model; the vision model is not on that wire.
	if strings.EqualFold(strings.TrimSpace(p.Kind), "responses") {
		return false
	}
	if !stringSlicesEqual(p.ModelList(), legacyDeepSeekV4Models) {
		return false
	}
	p.Models = append(append([]string(nil), legacyDeepSeekV4Models...), DeepSeekVisionModel)
	// Listed but unticked is the worst of both: the model is the one thing on
	// this endpoint that reads images, and picking it would silently drop them.
	p.VisionModels = migrateDeepSeekVisionModels(p.VisionModels)
	return true
}

// migrateDeepSeekVisionModels ticks the new model unless the user has said
// something about vision here. An empty list is what the renderer writes for
// "nothing to say", so it counts as unsaid; a non-empty one is a choice.
func migrateDeepSeekVisionModels(current []string) []string {
	if len(current) > 0 {
		return current
	}
	return []string{DeepSeekVisionModel}
}
