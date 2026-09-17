package boot

import (
	"path/filepath"
	"strings"

	"reasonix/internal/browser"
	"reasonix/internal/config"
	"reasonix/internal/tool"
	"reasonix/internal/tool/builtin"
	"reasonix/internal/workspaceid"
)

// browserPool is process-wide: controllers on one workspace share its
// profile, and a profile admits one browser process.
var browserPool = &browser.Pool{}

// withBrowser gives the browser tools a browser session when the browser is
// enabled, and returns cleanup extended to close it. Which browser is found at
// first use, so the schema answers to the config alone and a machine without
// one hears browser.engine_missing then.
func withBrowser(reg *tool.Registry, cfg config.BrowserConfig, root string, cleanup func()) func() {
	profiles := config.BrowserProfilesDir()
	if !cfg.Enabled || root == "" || profiles == "" {
		return cleanup
	}
	session := browser.NewSession(browser.Config{
		Launch: browser.LaunchSpec{
			Executable: cfg.Executable,
			ProfileDir: filepath.Join(profiles, strings.ReplaceAll(workspaceid.Key(root), ":", "-")),
			Headless:   cfg.Headless,
		},
		Roots: []string{root},
		Pool:  browserPool,
	})
	for _, t := range builtin.BrowserTools(session) {
		reg.Add(t)
	}
	return func() { cleanup(); session.Close() }
}
