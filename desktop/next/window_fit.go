//go:build desktop

package main

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// How much of the screen a fresh window may take. Proportional rather than a
// fixed margin: 64px of air is cramped on a 4K panel and generous on a laptop,
// while "most of it, not all of it" reads the same at every size.
const screenShare = 90

// fitted returns the size to open at: the preferred size treated as a ceiling,
// not a constant. The window is whatever is smaller — what we would like, or
// what this screen can comfortably show.
//
// A screen reporting nothing (some drivers do before the first paint) leaves
// the preference alone rather than collapsing the window to a share of zero.
func fitted(maxW, maxH, screenW, screenH int) (int, int) {
	if screenW > 0 {
		maxW = min(maxW, screenW*screenShare/100)
	}
	if screenH > 0 {
		maxH = min(maxH, screenH*screenShare/100)
	}
	return maxW, maxH
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
//
// The window starts hidden and is shown from here, so none of this is visible
// as a resize — it opens already the right size, in the right place.
func fitWindow(ctx context.Context) {
	// Whatever happens below, the window becomes visible. A measurement that
	// fails must cost a badly sized window, never an invisible one.
	defer runtime.WindowShow(ctx)

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
