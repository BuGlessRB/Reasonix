package anthropic

import (
	"context"
	"testing"

	"reasonix/internal/provider"
)

func relayClient(t *testing.T, base string, extra map[string]any) *client {
	t.Helper()
	p, err := New(provider.Config{
		Name:    "relay",
		BaseURL: base,
		Model:   "deepseek-v3.2",
		APIKey:  "test",
		Extra:   extra,
	})
	if err != nil {
		t.Fatalf("New relay: %v", err)
	}
	c, ok := p.(*client)
	if !ok {
		t.Fatalf("New returned %T, want *client", p)
	}
	return c
}

func replayTurn() provider.Request {
	return provider.Request{Messages: []provider.Message{
		{Role: provider.RoleUser, Content: "weather?"},
		{
			Role:             provider.RoleAssistant,
			ReasoningContent: "I should call the tool.",
			ToolCalls:        []provider.ToolCall{{ID: "t1", Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{Role: provider.RoleTool, ToolCallID: "t1", Content: "sunny"},
	}}
}

func assistantBlocks(t *testing.T, r anthRequest) []contentBlock {
	t.Helper()
	if len(r.Messages) < 2 {
		t.Fatalf("messages = %+v, want a replayed assistant turn", r.Messages)
	}
	return r.Messages[1].Content
}

// The contract this endpoint speaks was readable only off the host, so a relay
// carrying the same models had no way to ask for the unsigned thinking block it
// requires — and no setting could rescue it.
func TestDeclaredDeepSeekRelayReplaysUnsignedThinking(t *testing.T) {
	c := relayClient(t, "https://relay.example.com", map[string]any{"reasoning_protocol": "deepseek"})
	blocks := assistantBlocks(t, c.buildRequest(context.Background(), replayTurn()))

	if len(blocks) != 2 || blocks[0].Type != "thinking" || blocks[0].Thinking != "I should call the tool." {
		t.Fatalf("assistant blocks = %+v, want thinking before tool_use", blocks)
	}
	if blocks[0].Signature != "" {
		t.Errorf("DeepSeek issues no signature; sending one invents proof: %+v", blocks[0])
	}
}

func TestUndeclaredRelayDropsThinkingAndNamesIt(t *testing.T) {
	c := relayClient(t, "https://relay.example.com", nil)
	r := c.buildRequest(context.Background(), replayTurn())

	for _, b := range assistantBlocks(t, r) {
		if b.Type == "thinking" {
			t.Fatalf("an undeclared endpoint must not be sent thinking: %+v", b)
		}
	}
	if r.reasoningHint != provider.HintDroppedToolCallReasoning {
		t.Errorf("hint = %q, want %q", r.reasoningHint, provider.HintDroppedToolCallReasoning)
	}
}

// On Anthropic proper unsigned reasoning is unsendable rather than withheld:
// the endpoint requires a signature it alone issues. Reporting it as a
// declaration the user forgot would send them to a setting that cannot help.
func TestNativeAnthropicUnsignedReasoningIsNotReported(t *testing.T) {
	c := relayClient(t, "https://api.anthropic.com", map[string]any{"thinking": "adaptive"})
	if hint := c.buildRequest(context.Background(), replayTurn()).reasoningHint; hint != "" {
		t.Errorf("hint = %q, want none", hint)
	}
}
