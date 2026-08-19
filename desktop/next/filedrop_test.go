package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Asking for dropped paths must not cost the window its ordinary drop: the
// composer still reads the DOM event for the bytes a browser tab has instead of
// a path. DisableWebViewDrop is the one switch that silences it — on macOS by
// short-circuiting performDragOperation, on Linux by gtk_drag_dest_unset — and
// nothing here would notice until a drop stopped landing.
func TestFileDropKeepsTheWebviewsOwnDrop(t *testing.T) {
	dnd := dragAndDrop()
	if dnd == nil || !dnd.EnableFileDrop {
		t.Fatal("file drop is off; a turn cannot reference a dropped file without it")
	}
	if dnd.DisableWebViewDrop {
		t.Fatal("DisableWebViewDrop would take the dropped bytes away from the composer")
	}
}

// The paths are taken through the Wails runtime in the page, not relayed from
// here: a relay would report every drop in the window with nothing to say which
// element it landed on, and that answer only exists in the DOM.
func TestPathsAreNotRelayedFromTheShell(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "OnFileDrop") {
		t.Fatal("the shell subscribes to file drops; that channel says nothing about the drop target, so the page must be the subscriber")
	}
}

// The runtime offers to filter drops itself, hit-testing native drop
// coordinates against a CSS opt-in. The layout is in CSS pixels, so the two
// part ways under an interface zoom and the filter rejects drops that landed
// squarely on the composer — file drop broken on one machine and nowhere else.
// The page routes against the DOM; asking again puts the arithmetic back.
func TestTheRuntimeFilterIsNotAskedFor(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "frontend-next", "src", "port", "sse.ts"))
	if err != nil {
		t.Fatal(err)
	}
	call := strings.Index(string(src), "rt.OnFileDrop(")
	if call < 0 {
		t.Fatal("nothing subscribes to dropped paths, so a dropped file never reaches a turn")
	}
	stmt := string(src)[call:]
	end := strings.Index(stmt, ";")
	if end < 0 {
		t.Fatal("could not read the OnFileDrop call")
	}
	if !strings.HasSuffix(strings.TrimSuffix(strings.TrimSpace(stmt[:end]), ")"), "false") {
		t.Fatalf("OnFileDrop asks for the runtime's own drop-target filter; it hit-tests native coordinates against CSS pixels: %s", stmt[:end])
	}
}
