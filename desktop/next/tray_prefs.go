// tray_prefs.go — the status icon as a setting, answered by the window itself.
package main

import (
	"reasonix/internal/config"
)

// TrayPrefs is what the window can say about its own icon. Icon is what the
// setting asks for and Live is whether there is one right now: systray quits
// once per process, so taking the icon down mid-session would be a switch that
// cannot be switched back. The two differ only until the next launch, and a
// panel that shows both can say so instead of pretending.
type TrayPrefs struct {
	Icon        bool `json:"icon"`
	Live        bool `json:"live"`
	CloseToTray bool `json:"closeToTray"`
}

// TrayPrefs reports the current state to the settings pane.
func (a *App) TrayPrefs() TrayPrefs {
	return TrayPrefs{
		Icon:        config.LoadForEdit(config.UserConfigPath()).DesktopTray() != "off",
		Live:        a.iconLive(),
		CloseToTray: a.background.Load(),
	}
}

// SetTrayPrefs applies both switches and writes them down. Closing behaviour
// takes effect now — it is a flag, and the next close is the whole point of
// asking. The icon waits for a launch, which is the honest half: there is no
// way to put one back on this process.
func (a *App) SetTrayPrefs(icon, closeToTray bool) TrayPrefs {
	// No icon, no backgrounding. A hidden window with nothing to bring it back
	// is the one state this feature must never produce, so the answer depends
	// on an icon being both asked for and actually up.
	closeToTray = closeToTray && icon && a.iconLive()
	a.background.Store(closeToTray)

	path := config.UserConfigPath()
	edit := config.LoadForEdit(path)
	trayMode, behavior := "auto", "quit"
	if !icon {
		trayMode = "off"
	}
	if closeToTray {
		behavior = "background"
	}
	if err := edit.SetDesktopTray(trayMode); err == nil {
		if err := edit.SetDesktopCloseBehavior(behavior); err == nil {
			_ = edit.SaveTo(path)
		}
	}
	return a.TrayPrefs()
}

func (a *App) iconLive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.icon != nil
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
