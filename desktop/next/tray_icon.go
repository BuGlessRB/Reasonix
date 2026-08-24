// tray_icon.go — the status icon's three faces, drawn rather than shipped.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"reasonix/internal/traystate"
)

// The palette the window itself uses (styles/tokens.css). A status icon that
// invented its own gold would be the one place the product is a different
// colour, and the mood has to survive a 16-pixel render: idle is the same mark
// dimmed, and attention adds a badge, because a hue change alone is not
// something everyone can see.
var (
	markIdle       = color.NRGBA{R: 0x6B, G: 0x5A, B: 0x38, A: 0xFF}
	markActive     = color.NRGBA{R: 0xDD, G: 0xA1, B: 0x44, A: 0xFF}
	badgeAttention = color.NRGBA{R: 0xE4, G: 0x67, B: 0x5D, A: 0xFF}
)

// iconSize is drawn well above the 16px the trays ask for: every platform
// downscales, and none of them upscale kindly.
const iconSize = 64

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

// drawMark paints the mark: a rounded square, dim when nothing is happening,
// with a corner badge when a turn is waiting on the person who cannot see the
// window it is waiting in.
func drawMark(mood traystate.Mood) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	fill := markActive
	if mood == traystate.MoodIdle {
		fill = markIdle
	}
	roundedSquare(img, image.Rect(6, 6, iconSize-6, iconSize-6), iconSize/5, fill)
	// The notch is what makes the mark a mark rather than a coloured blob at
	// this size: one bite out of the middle, in the page colour behind it.
	roundedSquare(img, image.Rect(iconSize/2-4, 18, iconSize/2+4, iconSize-18), 4, color.NRGBA{R: 0x09, G: 0x0B, B: 0x0F, A: 0xFF})
	if mood == traystate.MoodAttention {
		disc(img, image.Pt(iconSize-16, 16), 13, color.NRGBA{R: 0x09, G: 0x0B, B: 0x0F, A: 0xFF})
		disc(img, image.Pt(iconSize-16, 16), 10, badgeAttention)
	}
	return img
}

func roundedSquare(dst *image.NRGBA, box image.Rectangle, radius int, fill color.NRGBA) {
	draw.Draw(dst, image.Rect(box.Min.X+radius, box.Min.Y, box.Max.X-radius, box.Max.Y), &image.Uniform{fill}, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(box.Min.X, box.Min.Y+radius, box.Max.X, box.Max.Y-radius), &image.Uniform{fill}, image.Point{}, draw.Src)
	for _, at := range []image.Point{
		{X: box.Min.X + radius, Y: box.Min.Y + radius},
		{X: box.Max.X - radius, Y: box.Min.Y + radius},
		{X: box.Min.X + radius, Y: box.Max.Y - radius},
		{X: box.Max.X - radius, Y: box.Max.Y - radius},
	} {
		disc(dst, at, radius, fill)
	}
}

// disc fills a circle with one row of alpha at the edge, which is all the
// antialiasing a mark this simple needs.
func disc(dst *image.NRGBA, at image.Point, radius int, fill color.NRGBA) {
	for y := at.Y - radius; y <= at.Y+radius; y++ {
		for x := at.X - radius; x <= at.X+radius; x++ {
			dx, dy := float64(x-at.X), float64(y-at.Y)
			d := dx*dx + dy*dy
			switch r := float64(radius); {
			case d <= (r-1)*(r-1):
				dst.SetNRGBA(x, y, fill)
			case d <= r*r:
				edge := fill
				edge.A = 0x9A
				dst.SetNRGBA(x, y, edge)
			}
		}
	}
}
