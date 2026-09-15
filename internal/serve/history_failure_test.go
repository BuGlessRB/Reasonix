package serve

import (
	"testing"

	"reasonix/internal/provider"
)

// A rebuilt card knows a call failed because the host said so, not because the
// words start with "error:" — a tool's own output can start that way, and a
// refusal's wording is not its identity.
func TestHistoryCarriesTheHostsAccountOfAFailure(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c1", Name: "use_capability"}}},
		{
			Role:        provider.RoleTool,
			ToolCallID:  "c1",
			Name:        "use_capability",
			Content:     "nothing about this sentence says it is a refusal",
			ToolFailure: &provider.ToolFailure{RefusalCode: "goal.no_active_turn"},
		},
	}
	var result historyMessage
	for _, hm := range historyMessages(msgs) {
		if hm.Role == "tool" {
			result = hm
		}
	}
	if !result.ToolFailed {
		t.Fatal("the rebuild lost the fact that the call failed")
	}
	if result.ToolRefusalCode != "goal.no_active_turn" {
		t.Fatalf("refusal identity = %q, want goal.no_active_turn", result.ToolRefusalCode)
	}
}

// A result that succeeded says nothing, so a card cannot read absence as failure.
func TestHistoryLeavesASuccessUnmarked(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c1", Name: "bash"}}},
		{Role: provider.RoleTool, ToolCallID: "c1", Name: "bash", Content: "error: this is grep output, not a refusal"},
	}
	for _, hm := range historyMessages(msgs) {
		if hm.Role == "tool" && (hm.ToolFailed || hm.ToolRefusalCode != "") {
			t.Fatalf("a successful result was marked failed from its words: %+v", hm)
		}
	}
}
