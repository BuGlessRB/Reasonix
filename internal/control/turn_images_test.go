package control

import (
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
	if !strings.Contains(got, "<"+imageRoutingTag+">") || !strings.Contains(got, "2 image(s)") {
		t.Fatalf("note = %q, want an attached-images block naming the count", got)
	}
	if !strings.Contains(got, "read_only_task") {
		t.Fatalf("note = %q, want it to name the delegated read", got)
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
