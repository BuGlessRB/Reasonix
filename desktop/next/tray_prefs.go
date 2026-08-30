// tray_prefs.go — what only this window can answer about its own status icon.
package main

import (
	"reasonix/internal/serve"
	"reasonix/internal/traystate"
)

// IconLive reports whether this launch got an icon. systray gives one up once
// per process, so a setting turned back on cannot put another there.
func (a *App) IconLive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.icon != nil
}

// TrayFold is what the panes add up to. Jobs are the hub's to count.
func (a *App) TrayFold() traystate.State {
	if a.tracker == nil {
		return traystate.State{}
	}
	return a.tracker.State()
}

// ApplyTrayPrefs takes effect on this launch: the close button answers the flag
// rather than re-reading the file, and the menu's checkbox has to agree with it.
func (a *App) ApplyTrayPrefs(prefs serve.TrayPrefs) {
	a.background.Store(prefs.CloseToTray)
	a.mu.Lock()
	icon := a.icon
	a.mu.Unlock()
	icon.showBackground(prefs.CloseToTray)
}

// adoptIcon records the icon this launch got, if any.
func (a *App) adoptIcon(t *tray) {
	a.mu.Lock()
	a.icon = t
	a.mu.Unlock()
}

// closeIcon takes it down at shutdown, which is the only time it comes down.
func (a *App) closeIcon() {
	a.mu.Lock()
	t := a.icon
	a.icon = nil
	a.mu.Unlock()
	t.close()
}
