//go:build desktop

package main

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Room left around the window when a screen cannot hold the default size. Not
// cosmetic: on Windows the title bar is drawn by the app (the frame is off), so
// a window taller than the screen puts its own title bar — and the drag region
// with it — above the top edge, where it cannot be grabbed to move the window
// back down.
const screenMargin = 64

// fitted returns the size to open at on a screen of the given logical size. A
// screen reporting nothing (some drivers do, on first paint) leaves the request
// untouched rather than collapsing the window to a margin.
func fitted(w, h, screenW, screenH int) (int, int) {
	if screenW > screenMargin && w > screenW-screenMargin {
		w = screenW - screenMargin
	}
	if screenH > screenMargin && h > screenH-screenMargin {
		h = screenH - screenMargin
	}
	return w, h
}

// fitWindow puts the window where it fits, on the screen it opened on.
//
// Sizes here are logical pixels, which is where this bites: a 1920x1080 display
// at Windows' common 125% scaling is 1536x864 to lay out in, and 150% leaves
// 1280x720. The default 1440x900 does not fit either, and centred in 864 its top
// edge lands at -18 — exactly the title bar. The report was from a 1080p screen,
// so the trap is not a small display, it is the scale factor between what the
// user reads off their monitor and what the window is measured in.
//
// Centring happens regardless: Wails does not promise a centred window on
// Windows, and a window that merely fits can still open partly off-screen.
func fitWindow(ctx context.Context) {
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		slog.Debug("studio: no screen geometry, keeping default window size", "err", err)
		return
	}
	screen := screens[0]
	for _, s := range screens {
		if s.IsCurrent {
			screen = s
			break
		}
	}
	// Logical pixels, because that is the space window sizes are set in.
	sw, sh := screen.Size.Width, screen.Size.Height
	if sw == 0 || sh == 0 {
		sw, sh = screen.Width, screen.Height
	}
	w, h := runtime.WindowGetSize(ctx)
	fw, fh := fitted(w, h, sw, sh)
	if fw != w || fh != h {
		slog.Info("studio: window did not fit the screen", "screen", []int{sw, sh}, "from", []int{w, h}, "to", []int{fw, fh})
		runtime.WindowSetSize(ctx, fw, fh)
	}
	runtime.WindowCenter(ctx)
}
