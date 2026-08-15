package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// Frameless on Windows takes the native minimise/maximise/close with it, so the
// chrome has to draw its own and these are what they call. macOS keeps its
// lights and never renders them; Linux keeps its whole frame.

func (a *App) MinimiseWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowMinimise(a.ctx)
}

func (a *App) ToggleMaximiseWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowToggleMaximise(a.ctx)
}

// IsWindowMaximised backs the restore/maximise glyph swap.
func (a *App) IsWindowMaximised() bool {
	if a.ctx == nil {
		return false
	}
	return runtime.WindowIsMaximised(a.ctx)
}

func (a *App) CloseWindow() {
	if a.ctx == nil {
		return
	}
	runtime.Quit(a.ctx)
}
