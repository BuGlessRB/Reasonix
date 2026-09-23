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

// bindBrowser gives the browser tools a session when the browser is enabled:
// reused, the one the replaced runtime had, otherwise a new one. Which browser
// runs is found at first use, so the schema answers to the config alone and a
// machine without one hears browser.engine_missing then.
func bindBrowser(reg *tool.Registry, cfg config.BrowserConfig, root string, reused *browser.Session) *browser.Session {
	profiles := config.BrowserProfilesDir()
	if !cfg.Enabled || root == "" || profiles == "" {
		return nil
	}
	session := reused
	if session == nil {
		session = browser.NewSession(browser.Config{
			Launch: browser.LaunchSpec{
				Executable: cfg.Executable,
				ProfileDir: filepath.Join(profiles, strings.ReplaceAll(workspaceid.Key(root), ":", "-")),
				Headless:   cfg.Headless,
			},
			Roots: []string{root},
			Pool:  browserPool,
		})
	}
	for _, t := range builtin.BrowserTools(session) {
		reg.Add(t)
	}
	return session
}

// SetBrowserHost routes every browser this process starts through a window
// that draws them, instead of launching one.
func SetBrowserHost(dial browser.EndpointDialer) { browserPool.SetEndpoint(dial) }

// renderRoot is where the executor opens a page it wrote to look at it: the
// workspace, when there is a browser to open it in and a model that can see the
// screenshot. Either missing, a look is nothing it could owe.
func renderRoot(session *browser.Session, entry *config.ProviderEntry, root string) string {
	if session == nil || entry == nil || !config.EffectiveVision(entry) {
		return ""
	}
	return root
}
