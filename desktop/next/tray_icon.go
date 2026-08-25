// tray_icon.go — the status icon: the composer's R, in one of three colours.
package main

import (
	"image"
	"image/color"
	"math"
	"sync"

	"reasonix/internal/traystate"
)

// One hue per mood and nothing else: the window's palette already gives each of
// the three a single meaning — accent is "needs you", net is "running on its
// own", ghost is neutral. The lightness belongs to neither theme, because a
// notification area is a background this process cannot ask about: these clear
// 3:1 against both white and near-black.
var moodInk = map[traystate.Mood]color.NRGBA{
	traystate.MoodIdle:      {R: 0x82, G: 0x7C, B: 0x74, A: 0xFF},
	traystate.MoodWorking:   {R: 0x50, G: 0x79, B: 0xD3, A: 0xFF},
	traystate.MoodAttention: {R: 0xAD, G: 0x72, B: 0x27, A: 0xFF},
}

// iconSize is drawn well above the 16px the trays ask for: every platform
// downscales, and none of them upscale kindly. markMargin is what stays clear
// on the tight side, since a menu bar scales the image to its own height.
const (
	iconSize   = 64
	markMargin = 6
)

// The mark is the composer's, kept in the 16-unit space RMark.tsx draws it in so
// the two can be read side by side rather than compared by eye.
const markStroke = 1.7

var markStrokes = [...][4]float64{
	{4.7, 2.9, 4.7, 13.1}, // stem
	{4.7, 2.9, 8.9, 2.9},  // the bowl's top
	{8.9, 8.7, 4.7, 8.7},  // and its bottom
	{9, 8.7, 12.3, 13.1},  // leg
}

// markBowl is the SVG's `a 2.9 2.9 0 0 1 0 5.8` resolved: the endpoints are one
// diameter apart, so the arc is the half circle between them, and sweep 1 puts
// that half to the right of the stem.
var markBowl = struct{ cx, cy, r float64 }{8.9, 5.8, 2.9}

var (
	iconOnce  sync.Once
	iconBytes map[traystate.Mood][]byte
)

// moodIcon returns the platform's icon bytes for one mood, drawn once.
func moodIcon(mood traystate.Mood) []byte {
	iconOnce.Do(func() {
		iconBytes = map[traystate.Mood][]byte{}
		for _, m := range []traystate.Mood{traystate.MoodIdle, traystate.MoodWorking, traystate.MoodAttention} {
			iconBytes[m] = encodeIcon(drawMark(m))
		}
	})
	return iconBytes[mood]
}

// drawMark paints the R at one mood's colour.
func drawMark(mood traystate.Mood) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	ink := moodInk[mood]
	scale, offX, offY := markFit()
	half := markStroke / 2 * scale
	for y := range iconSize {
		for x := range iconSize {
			away := markDistance((float64(x)+0.5-offX)/scale, (float64(y)+0.5-offY)/scale) * scale
			// One pixel of falloff across the edge is all the antialiasing a
			// shape of four strokes and an arc needs.
			cover := math.Max(0, math.Min(1, half-away+0.5))
			if cover == 0 {
				continue
			}
			lit := ink
			lit.A = uint8(cover * 255)
			img.SetNRGBA(x, y, lit)
		}
	}
	return img
}

// markFit maps glyph units onto icon pixels: the ink centred, scaled until the
// tight side reaches the margin, aspect kept.
func markFit() (scale, offX, offY float64) {
	minX, minY, maxX, maxY := markInk()
	w, h := maxX-minX, maxY-minY
	scale = math.Min((iconSize-2*markMargin)/w, (iconSize-2*markMargin)/h)
	return scale, (iconSize-w*scale)/2 - minX*scale, (iconSize-h*scale)/2 - minY*scale
}

// markInk is the box the painted glyph occupies — the skeleton grown by half a
// stroke, which is where the round caps end.
func markInk() (minX, minY, maxX, maxY float64) {
	half := markStroke / 2
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	grow := func(x, y float64) {
		minX, minY = math.Min(minX, x-half), math.Min(minY, y-half)
		maxX, maxY = math.Max(maxX, x+half), math.Max(maxY, y+half)
	}
	for _, s := range markStrokes {
		grow(s[0], s[1])
		grow(s[2], s[3])
	}
	grow(markBowl.cx+markBowl.r, markBowl.cy)
	return minX, minY, maxX, maxY
}

// markDistance measures a point in glyph units against the skeleton. Reading a
// stroke as a capsule is what gives the round caps and round joins the SVG asks
// for without drawing either.
func markDistance(x, y float64) float64 {
	best := bowlDistance(x, y)
	for _, s := range markStrokes {
		best = math.Min(best, segmentDistance(x, y, s[0], s[1], s[2], s[3]))
	}
	return best
}

func segmentDistance(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := 0.0
	if span := dx*dx + dy*dy; span > 0 {
		t = math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/span))
	}
	return math.Hypot(x-(ax+t*dx), y-(ay+t*dy))
}

// bowlDistance answers for the half circle only where that half is. Left of the
// centre the nearest ink is one of its two ends, and the strokes that meet the
// arc there already own those.
func bowlDistance(x, y float64) float64 {
	if x < markBowl.cx {
		return math.Inf(1)
	}
	return math.Abs(math.Hypot(x-markBowl.cx, y-markBowl.cy) - markBowl.r)
}
