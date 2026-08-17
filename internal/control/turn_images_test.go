package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/event"
)

// A pasted screenshot that reaches a text-only model must not vanish. The data
// is already carried as a sub-agent candidate; what was missing is any sign of
// it — so the model answered as though nothing had been attached.
func TestUnreadableImagesRideTheTurnTail(t *testing.T) {
	c := New(Options{Sink: event.Discard})
	got := c.imageRoutingPrefix(unreadableImages(nil, []string{"data:image/png;base64,AAA", "data:image/png;base64,BBB"})) + "look at this"
	if !strings.HasSuffix(got, "look at this") {
		t.Fatalf("the user's text must stay last in the turn: %q", got)
	}
	if !strings.Contains(got, "<"+ImageRoutingTag+">") || !strings.Contains(got, "2 image(s)") {
		t.Fatalf("note = %q, want an attached-images block naming the count", got)
	}
	// No vision model is configured here, so promising a delegate that also
	// cannot read would send the model down a path with nothing at the end.
	if !strings.Contains(got, "no vision model is configured") {
		t.Fatalf("note = %q, want it to say no reader is configured", got)
	}
}

// With a reader configured the note names it and closes the door on self-OCR —
// the two together are what stopped the model writing its own OCR script while
// the vision model sat unused.
func TestConfiguredVisionModelIsNamedInTheNote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[agent]\nvision_model = \"looker/eyes\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{Sink: event.Discard, WorkspaceRoot: t.TempDir()})
	got := c.imageRoutingNote(1)
	if !strings.Contains(got, "read_only_task") {
		t.Fatalf("note = %q, want it to name the delegated read", got)
	}
	if !strings.Contains(got, "looker/eyes") {
		t.Fatalf("note = %q, want it to name the model that reads", got)
	}
	if !strings.Contains(got, "Do not OCR") {
		t.Fatalf("note = %q, want self-OCR ruled out when a reader exists", got)
	}
}

// A vision model gets the images themselves, so the note would be noise — and
// worse, it would tell the model it cannot see what it can.
func TestReadableImagesGetNoNote(t *testing.T) {
	c := New(Options{Sink: event.Discard})
	imgs := []string{"data:image/png;base64,AAA"}
	if got := c.imageRoutingPrefix(unreadableImages(imgs, imgs)) + "look"; got != "look" {
		t.Fatalf("got %q, want the input untouched when the model reads images", got)
	}
}

func TestNoAttachmentsChangeNothing(t *testing.T) {
	c := New(Options{Sink: event.Discard})
	if got := c.imageRoutingPrefix(unreadableImages(nil, nil)) + "plain"; got != "plain" {
		t.Fatalf("got %q, want the input untouched with no attachments", got)
	}
}
