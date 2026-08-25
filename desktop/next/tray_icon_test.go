package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"reasonix/internal/traystate"
)

// The tray's R and the composer's are one drawing in two rasterizers, and only
// the SVG can be edited by eye. These are the numbers tray_icon.go's strokes
// were derived from, so a move over there fails here and asks for the same
// move — rather than leaving the icon quietly drawing the previous logo.
func TestTrayMarkTracksTheComposersR(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "frontend-next", "src", "ui", "RMark.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"M4.7 2.9V13.1",
		"M4.7 2.9h4.2a2.9 2.9 0 0 1 0 5.8H4.7",
		"M9 8.7l3.3 4.4",
	}
	var got []string
	for _, m := range regexp.MustCompile(`d="([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		got = append(got, m[1])
	}
	if len(got) != len(want) {
		t.Fatalf("RMark.tsx draws %d paths, tray_icon.go was derived from %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path %d is now %q, was %q — re-derive markStrokes/markBowl from it", i, got[i], want[i])
		}
	}
}

// Same for the weight: the stroke is a CSS declaration on one side and a
// constant on the other.
func TestTrayMarkStrokeMatchesTheComposers(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "frontend-next", "src", "styles", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`\.rmark path \{[^}]*stroke-width:\s*([0-9.]+)`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("app.css no longer gives .rmark path a stroke-width; this test cannot see the weight")
	}
	width, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	if width != markStroke {
		t.Errorf("composer draws the R at %v, tray at %v", width, markStroke)
	}
}

// A mood the icon renders the same as another is a status nobody can read, and
// the icon is the only surface a hidden window has.
func TestEveryMoodRendersItsOwnIcon(t *testing.T) {
	seen := map[string]traystate.Mood{}
	for _, mood := range []traystate.Mood{traystate.MoodIdle, traystate.MoodWorking, traystate.MoodAttention} {
		encoded := moodIcon(mood)
		if len(encoded) == 0 {
			t.Fatalf("mood %v encodes to nothing", mood)
		}
		if other, dup := seen[string(encoded)]; dup {
			t.Errorf("moods %v and %v render identically", other, mood)
		}
		seen[string(encoded)] = mood
	}
}

// A menu bar scales the image to its own height, so ink at the edge is ink that
// gets clipped; a glyph that does not reach the margin is one nobody can make
// out at 16px.
func TestTrayMarkFillsTheIconWithoutTouchingTheEdge(t *testing.T) {
	ink := alphaBounds(drawMark(traystate.MoodIdle))
	if ink.Min.X < markMargin-1 || ink.Min.Y < markMargin-1 ||
		ink.Max.X > iconSize-markMargin+1 || ink.Max.Y > iconSize-markMargin+1 {
		t.Errorf("ink %v leaves the %dpx margin of a %dpx icon", ink, markMargin, iconSize)
	}
	if height := ink.Dy(); height < iconSize-2*markMargin-1 {
		t.Errorf("ink is %dpx tall in a %dpx icon; the margin is %d", height, iconSize, markMargin)
	}
}
