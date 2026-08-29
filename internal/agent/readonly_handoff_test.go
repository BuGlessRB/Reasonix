package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// A writer child submits a typed claim the host adjudicates. A read-only child
// used to end in prose, so what it found reached the parent as an unverifiable
// sentence while its receipts sat elsewhere in the host ledger.

func TestCompleteSubtaskSurvivesTheStrictReadOnlyFilter(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(subagentRegistryTool{name: "read_file", readOnly: true})
	AttachCompleteSubtaskTool(reg)

	// The child's registry is filtered once more at construction; a protocol
	// tool that did not survive would leave the contract asking for a call the
	// child cannot make.
	strict := strictReadOnlyExecutionRegistry(reg)
	if _, ok := strict.Get("complete_subtask"); !ok {
		t.Fatalf("complete_subtask did not survive strict read-only construction: %v", strict.Names())
	}
}

// The protocol tool is not a capability. Attaching it before the "can this
// child investigate anything?" check would let a child with no research tools
// pass as having one, and it would then report on work it could not do.
func TestReportingToolDoesNotCountAsAResearchCapability(t *testing.T) {
	parent := tool.NewRegistry()
	parent.Add(subagentRegistryTool{name: "write_file"}) // not read-only

	sub := ReadOnlySubagentToolRegistry(parent, nil)
	if sub.Len() != 0 {
		t.Fatalf("a parent with no read-only tools produced %v", sub.Names())
	}
	// And once it is attached, the registry is no longer empty — which is
	// exactly why the emptiness check has to come first.
	AttachCompleteSubtaskTool(sub)
	if sub.Len() == 0 {
		t.Fatal("complete_subtask was not attached at all")
	}
}

// Both kinds of child are told the same closing protocol, and the wording has
// to fit a child that only looked at things.
func TestCompletionContractCoversInspectionNotOnlyChange(t *testing.T) {
	if !strings.Contains(completeSubtaskContract, "complete_subtask exactly once") {
		t.Error("the contract no longer names the call it requires")
	}
	if !strings.Contains(completeSubtaskContract, "inspected") {
		t.Errorf("the contract asks only about changes, so a read-only child has nothing to cite:\n%s",
			completeSubtaskContract)
	}
}

// The shadow answers "did the child close the way it was asked to", so its
// denominator must be the instruction the host actually gave.
func TestHandoffShadowCountsAttemptsApartFromAcceptance(t *testing.T) {
	audit := event.SubagentHandoffAudit{}
	countReportCalls(&audit, []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "a", Name: "read_file", Arguments: `{"path":"x"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "a", Name: "read_file", Content: "ok"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "r1", Name: completeSubtaskToolName, Arguments: `{}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "r1", Name: completeSubtaskToolName, Content: "error: invalid status"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "r2", Name: completeSubtaskToolName, Arguments: `{"status":"complete"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "r2", Name: completeSubtaskToolName, Content: "recorded"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "b", Name: "read_file", Arguments: `{"path":"y"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "b", Name: "read_file", Content: "ok"},
	})

	// A refused call is a schema problem; never calling is a protocol one, and
	// one count cannot say which happened.
	if audit.Attempts != 2 || audit.Accepted != 1 || audit.Malformed != 1 {
		t.Errorf("attempts=%d accepted=%d malformed=%d, want 2/1/1",
			audit.Attempts, audit.Accepted, audit.Malformed)
	}
	// The contract asks for the report as the final call. Work after it is a
	// different behaviour from submitting one at all.
	if audit.ToolCallsAfterReport != 1 {
		t.Errorf("tool calls after the report = %d, want 1", audit.ToolCallsAfterReport)
	}
	if audit.ReportRound != 2 || audit.FinalRound != 4 {
		t.Errorf("rounds = report %d final %d, want 2 and 4", audit.ReportRound, audit.FinalRound)
	}
}

// A run that died on the provider never had the chance to comply. Counting it
// as a missing report would make the adoption rate answer a different question.
func TestHandoffShadowSeparatesFailureFromNonCompliance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		err    error
		want   string
	}{
		{"clean", "here is what I found", nil, "completed"},
		{"provider failed", "", errors.New("stream error"), "error"},
		// A wrapped sentinel, not a lookalike string: identity is what the
		// classifier reads, so a message that merely says so must not pass.
		{"user cancelled", "", fmt.Errorf("sub-agent: %w", context.Canceled), "cancelled"},
		{"message only says cancelled", "", errors.New("context canceled"), "error"},
		{"ran but said nothing", "  ", nil, "no_answer"},
	} {
		if got := handoffExit(tc.answer, tc.err); got != tc.want {
			t.Errorf("%s: exit = %q, want %q", tc.name, got, tc.want)
		}
	}
}
