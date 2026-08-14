// deepseek_canonical.go — folding the official DeepSeek source into one entry.
package config

import "strings"

// canonicalizeOfficialDeepSeekSource folds deepseek-flash and deepseek-pro into
// the one "deepseek" entry carrying both models: one endpoint, one key, one
// account, which a panel listing sources otherwise shows as two. A curated model
// list is left alone — the merged entry carries both models and would re-add the
// one the user removed.
func canonicalizeOfficialDeepSeekSource(c *Config) bool {
	if c == nil || !canCanonicalizeLegacyDeepSeekProviders(c) {
		return false
	}
	legacy := officialLegacyDeepSeekProviders(c)
	if len(legacy) == 0 {
		return false
	}
	drop := make(map[string]bool, len(legacy))
	for _, p := range legacy {
		if len(p.Models) > 0 {
			return false
		}
		drop[p.Name] = true
	}
	ensureDeepSeekOfficialProvider(c)
	if p, ok := c.Provider("deepseek"); !ok || officialProviderKind(p) != "deepseek" {
		return false
	}
	// The canonical entry takes the position of the first entry it replaces.
	// Appending it instead would reorder the list, and provider order decides
	// which entry answers a bare model name.
	var canonical ProviderEntry
	at := -1
	kept := make([]ProviderEntry, 0, len(c.Providers))
	for _, p := range c.Providers {
		if p.Name != "deepseek" && !drop[p.Name] {
			kept = append(kept, p)
			continue
		}
		if p.Name == "deepseek" {
			canonical = p
		}
		if at < 0 {
			at = len(kept)
		}
	}
	kept = append(kept, ProviderEntry{})
	copy(kept[at+1:], kept[at:])
	kept[at] = canonical
	c.Providers = kept
	foldDesktopProviderAccess(c, drop)
	return true
}

// refForFoldedDeepSeek maps a ref naming a folded entry onto the canonical one.
// Rewriting every ref in the config instead would make Default().DefaultModel
// and a loaded one disagree, and would not reach the refs that live outside the
// file anyway — shell history, scripts, a --model flag someone has memorised.
func (c *Config) refForFoldedDeepSeek(ref string) string {
	provider, _, _ := strings.Cut(ref, "/")
	if provider != "deepseek-flash" && provider != "deepseek-pro" {
		return ref
	}
	if _, stillConfigured := c.Provider(provider); stillConfigured {
		return ref
	}
	if _, folded := c.Provider("deepseek"); !folded {
		return ref
	}
	return retargetDesktopOfficialRef(ref, map[string]bool{"deepseek": true})
}

// foldDesktopProviderAccess points the desktop access list at the surviving
// name. Both folded names collapse to one entry, so it dedupes as it goes.
func foldDesktopProviderAccess(c *Config, drop map[string]bool) {
	if len(c.Desktop.ProviderAccess) == 0 {
		return
	}
	seen := make(map[string]bool, len(c.Desktop.ProviderAccess))
	out := make([]string, 0, len(c.Desktop.ProviderAccess))
	for _, raw := range c.Desktop.ProviderAccess {
		name := strings.TrimSpace(raw)
		if drop[name] {
			name = "deepseek"
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	c.Desktop.ProviderAccess = out
}
