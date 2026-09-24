package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BrowserConfig is the browser the agent drives. Executable, when set, is the
// only browser used; otherwise an installed Chrome, Edge or Chromium is found.
type BrowserConfig struct {
	Enabled    bool   `toml:"enabled"`
	Executable string `toml:"executable"`
	Headless   bool   `toml:"headless"`
}

// BrowserProfilesDir holds one browser profile per workspace — the logins and
// cookies the agent's browser keeps, apart from the person's own browser.
func BrowserProfilesDir() string {
	home := processRoots().Home()
	if strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "browser")
}

func renderBrowserConfig(b *strings.Builder, cfg BrowserConfig) {
	b.WriteString("[browser]\n")
	fmt.Fprintf(b, "enabled = %v   # browser tools; the browser starts on first use\n", cfg.Enabled)
	if cfg.Executable != "" {
		fmt.Fprintf(b, "executable = %q   # empty = find Chrome, Edge or Chromium\n", cfg.Executable)
	} else {
		b.WriteString("# executable = \"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome\"\n")
	}
	fmt.Fprintf(b, "headless = %v   # true = no window\n\n", cfg.Headless)
}

// renderChangedBrowserConfig writes [browser] only where it differs from base.
func renderChangedBrowserConfig(b *strings.Builder, cfg, base BrowserConfig) {
	if cfg != base {
		renderBrowserConfig(b, cfg)
	}
}
