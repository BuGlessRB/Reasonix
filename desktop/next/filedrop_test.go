package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Asking for dropped paths must not cost the window its ordinary drop. The
// composer receives pasted and dropped images through the DOM event, and
// DisableWebViewDrop is the one switch that would silence it — on macOS by
// short-circuiting performDragOperation, on Linux by gtk_drag_dest_unset.
// Nothing else in this repository would notice until an image stopped landing.
func TestFileDropKeepsTheWebviewsOwnDrop(t *testing.T) {
	dnd := dragAndDrop()
	if dnd == nil || !dnd.EnableFileDrop {
		t.Fatal("file drop is off; an installer cannot resolve a dropped folder without it")
	}
	if dnd.DisableWebViewDrop {
		t.Fatal("DisableWebViewDrop would take dropped images away from the composer")
	}
}

// The paths are taken through the Wails runtime in the page, not relayed from
// here: only that subscription filters by drop target, and a relay would report
// every drop in the window — including an image meant for the composer.
func TestPathsAreNotRelayedFromTheShell(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "OnFileDrop") {
		t.Fatal("the shell subscribes to file drops; that channel is unfiltered, so the page must be the subscriber")
	}
}

// A path only reaches the callback when the drop lands on an element that opted
// in, so the opt-in has to exist somewhere. Without it the feature is wired end
// to end and silently never fires.
func TestSomethingOptsInAsADropTarget(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "frontend-next", "src", "styles", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "--wails-drop-target: drop") {
		t.Fatal("no element declares --wails-drop-target: drop, so no drop is ever reported")
	}
}
