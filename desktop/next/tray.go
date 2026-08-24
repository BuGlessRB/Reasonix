// tray.go — the status icon, which is the only surface a hidden window has.
package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"reasonix/internal/config"
	"reasonix/internal/i18n"
	"reasonix/internal/traystate"
)

// tray owns the icon and the menu under it. It reports the fold and nothing
// else: every action it offers is a verb the window already has, so there is
// one implementation of "quit" and one of "come back".
type tray struct {
	shell *App
	say   i18n.Messages
	stop  func()

	done     chan struct{}
	mu       sync.Mutex
	status   *systray.MenuItem
	stayOpen *systray.MenuItem
	last     traystate.State
}

// jobPollInterval is how often the job count is refreshed. A job announces its
// life in a sentence and nowhere else, so this asks rather than listens, and a
// status line is not a scheduler.
const jobPollInterval = 5 * time.Second

// startTray brings the icon up beside the window's own loop and reports whether
// there is one. A shell with no icon must never background its window: hiding
// it would leave a running session with no way back to it.
func startTray(shell *App, say i18n.Messages, tracker *traystate.Tracker, cfg *config.Config) *tray {
	if cfg != nil && cfg.DesktopTray() == "off" {
		return nil
	}
	t := &tray{shell: shell, say: say, done: make(chan struct{})}
	t.stop = runTray(t.ready, func() {})
	tracker.SetOnChange(t.show)
	go t.pollJobs(tracker)
	return t
}

// pollJobs keeps the count of what the window would leave behind fresh.
func (t *tray) pollJobs(tracker *traystate.Tracker) {
	tick := time.NewTicker(jobPollInterval)
	defer tick.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-tick.C:
			tracker.SetJobs(t.runningJobs())
		}
	}
}

// runningJobs counts this machine's own. A remote pane's jobs live on the far
// kernel, and answering for them from here would be a number nobody could act
// on from this menu.
func (t *tray) runningJobs() int {
	if t.shell == nil || t.shell.hub == nil {
		return 0
	}
	running := 0
	for _, rt := range t.shell.hub.Runtimes() {
		if rt == nil || rt.Server == nil || !rt.Local() {
			continue
		}
		running += len(rt.Server.Controller().Jobs())
	}
	return running
}

// ready builds the menu once the platform has an icon to hang it on.
func (t *tray) ready() {
	systray.SetIcon(moodIcon(traystate.MoodIdle))
	systray.SetTitle("Reasonix")
	systray.SetTooltip("Reasonix Studio")
	systray.SetOnTapped(t.shell.showWindow)

	open := systray.AddMenuItem(t.say.TrayOpen, "")
	systray.AddSeparator()
	status := systray.AddMenuItem(t.say.TrayIdle, "")
	status.Disable()
	stay := systray.AddMenuItemCheckbox(t.say.TrayCloseToTray, "", t.shell.background.Load())
	systray.AddSeparator()
	quit := systray.AddMenuItem(t.say.TrayQuit, "")

	t.mu.Lock()
	t.status, t.stayOpen = status, stay
	last := t.last
	t.mu.Unlock()
	t.show(last)

	go t.pump(open.ClickedCh, stay.ClickedCh, quit.ClickedCh)
}

// pump answers the menu. One goroutine for the three channels, because a click
// is a rare event and three of them would only be three ways to leak.
func (t *tray) pump(open, stay, quit <-chan struct{}) {
	for {
		select {
		case _, ok := <-open:
			if !ok {
				return
			}
			t.shell.showWindow()
		case _, ok := <-stay:
			if !ok {
				return
			}
			t.toggleBackground()
		case _, ok := <-quit:
			if !ok {
				return
			}
			t.shell.quitFromTray()
			return
		}
	}
}

// toggleBackground flips what the close button does and writes it down. The
// runtime flag moves first: the answer to the next close is what the person
// just asked for, whether or not the config file can be written.
func (t *tray) toggleBackground() {
	want := !t.shell.background.Load()
	t.shell.background.Store(want)
	t.mu.Lock()
	item := t.stayOpen
	t.mu.Unlock()
	if item != nil {
		if want {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
	behavior := "quit"
	if want {
		behavior = "background"
	}
	path := config.UserConfigPath()
	edit := config.LoadForEdit(path)
	if err := edit.SetDesktopCloseBehavior(behavior); err == nil {
		_ = edit.SaveTo(path)
	}
}

// show paints one fold. It is called from whichever goroutine emitted the event
// that changed it, so it touches only the icon and its own items.
func (t *tray) show(state traystate.State) {
	t.mu.Lock()
	t.last = state
	status := t.status
	t.mu.Unlock()
	systray.SetIcon(moodIcon(state.Mood()))
	line := t.line(state)
	systray.SetTooltip("Reasonix Studio — " + line)
	if status != nil {
		status.SetTitle(line)
	}
}

// line says the fold in the reader's language. Background jobs are the half
// nobody would guess: a hidden window with a dev server in it is still doing
// something on the machine's behalf, so it is named even when no turn is.
func (t *tray) line(state traystate.State) string {
	var parts []string
	if state.Attention > 0 {
		parts = append(parts, fmt.Sprintf(t.say.TrayAttention, state.Attention))
	}
	if state.Working > 0 {
		parts = append(parts, fmt.Sprintf(t.say.TrayWorking, state.Working))
	}
	if state.Jobs > 0 {
		parts = append(parts, fmt.Sprintf(t.say.TrayJobs, state.Jobs))
	}
	if len(parts) == 0 {
		return t.say.TrayIdle
	}
	return strings.Join(parts, " · ")
}

// close takes the icon down. The window's own shutdown owns the order: the icon
// outlives every pane so the last thing on screen is still telling the truth.
func (t *tray) close() {
	if t == nil || t.stop == nil {
		return
	}
	close(t.done)
	t.stop()
}
