// provider_keyreuse.go — a second door onto an account already signed in.
package serve

import (
	"strings"

	"reasonix/internal/config"
)

// keyEnvForNewSource decides which credential slot a newly added source writes.
// A blank key at a host that already has one means another door onto that
// account, so the two share a slot — a second slot would leave the new entry
// unauthenticated and the pair reading as two accounts. A supplied key is a
// different account there, and keeps its own slot so it overwrites nothing.
func keyEnvForNewSource(name, baseURL, apiKey string) string {
	own := providerKeyEnv(name)
	if strings.TrimSpace(apiKey) != "" {
		return own
	}
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return own
	}
	host := vendorOf(baseURL)
	if host == "" {
		return own
	}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if vendorOf(p.BaseURL) == host && strings.TrimSpace(p.APIKeyEnv) != "" {
			return p.APIKeyEnv
		}
	}
	return own
}
