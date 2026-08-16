//go:build desktop

package main

import "testing"

// The failure this guards is not "the window is a bit big": on Windows the app
// draws its own title bar, so a window taller than the screen puts that bar —
// and the only region that can drag the window — above the top edge.
func TestFittedKeepsTheWindowOnScreen(t *testing.T) {
	for _, tc := range []struct {
		name         string
		w, h, sw, sh int
		wantW, wantH int
	}{
		// 报障的那台：物理 1920x1080，Windows 125% 缩放后可布局的是 1536x864
		{"1080p 屏在 125% 缩放下只剩 1536x864", 1440, 900, 1536, 864, 1382, 777},
		{"同一块屏 150% 缩放只剩 1280x720", 1440, 900, 1280, 720, 1152, 648},
		{"1366x768 的笔记本", 1440, 900, 1366, 768, 1229, 691},
		{"大屏上按上限来，不铺满", 1440, 900, 2560, 1440, 1440, 900},
		{"只有高度放不下", 1200, 900, 1920, 800, 1200, 720},
		{"屏幕尺寸报不出来时不动", 1440, 900, 0, 0, 1440, 900},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h := fitted(tc.w, tc.h, tc.sw, tc.sh)
			if w != tc.wantW || h != tc.wantH {
				t.Errorf("fitted(%d,%d on %dx%d) = %d,%d; want %d,%d", tc.w, tc.h, tc.sw, tc.sh, w, h, tc.wantW, tc.wantH)
			}
			if tc.sw > 0 && w > tc.sw {
				t.Errorf("still wider than the screen: %d > %d", w, tc.sw)
			}
			if tc.sh > 0 && h > tc.sh {
				t.Errorf("still taller than the screen: %d > %d", h, tc.sh)
			}
		})
	}
}
