package agent

import (
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// Delegation spawns work rather than changing state, so a failed one does not
// open the dependency barrier — batchCallIsMutatingFailure exempts it. Being
// skipped BY the barrier is the same question, and a bare !ReadOnly answers it
// the other way, dropping every delegation left in the batch.
func TestBatchDoesNotSkipDelegationAsADependentModification(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(&TaskTool{})
	a := &Agent{svc: agentServices{sink: event.Discard, tools: reg}}

	for _, name := range []string{"task"} {
		call := provider.ToolCall{Name: name, Arguments: `{"prompt":"do a thing","description":"d"}`}
		if batchCallStaticallySkippable(a, call) {
			t.Errorf("%s is skippable as a dependent modification, but a failed %s does not open the barrier either",
				name, name)
		}
	}
}
