package agent

import (
	"encoding/json"
	"strings"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// What the provider produced on its own side, rather than through the tool loop:
// an opaque item to replay next turn, or a tool it ran itself.

func appendProviderItem(items []json.RawMessage, item json.RawMessage) []json.RawMessage {
	if len(item) == 0 {
		return items
	}
	return append(items, append(json.RawMessage(nil), item...))
}

// absorbProviderRunCall reports a call that arrives already finished and keeps
// its result in the turn's text — the only copy the next turn has, because the
// provider keeps the plain content encrypted. Result and no dispatch: there is no
// pending moment to show, and a dispatch is the one event deferredStreamSink
// holds back, so it would land after its own result and never settle the card.
func absorbProviderRunCall(text *strings.Builder, sink event.Sink, chunk provider.Chunk, attemptID string) {
	text.WriteString(chunk.Text)
	tc := chunk.ToolCall
	if tc == nil {
		return
	}
	sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
		ID: tc.ID, Name: tc.Name, Args: tc.Arguments,
		Output: chunk.Text, ReadOnly: true, AttemptID: attemptID,
	}})
}
