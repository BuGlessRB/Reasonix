package plugin

import (
	"sync"
	"testing"

	"reasonix/internal/event"
)

type capturedSink struct {
	mu   sync.Mutex
	kind []event.Kind
	text []string
}

func (c *capturedSink) Emit(e event.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kind = append(c.kind, e.Kind)
	c.text = append(c.text, e.Text)
}

// A status view refreshes on an event, so a state change with no event is a
// view that stays wrong. The case that shipped: a tools-only server (no prompts,
// no resources) connects lazily on its first call, and the only announcements
// were made from the prompt and resource paths — which such a server never
// takes. It showed its boot-time failure while the agent used it fine.
func TestAnnounceReportsWithoutPromptsOrResources(t *testing.T) {
	h := NewHost()
	sink := &capturedSink{}
	h.SetStatusSink(sink)

	h.announce("%s: connected", "windows-mcp")

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.kind) != 1 || sink.kind[0] != event.MCPSurfaceReady {
		t.Fatalf("kinds = %v, want one MCPSurfaceReady", sink.kind)
	}
	if sink.text[0] != "windows-mcp: connected" {
		t.Errorf("text = %q", sink.text[0])
	}
}

// Assembly may hand over a nil sink (headless paths do), and a status change
// must not take the connection down with it.
func TestAnnounceToleratesNoSink(t *testing.T) {
	NewHost().announce("x: connected")
}
