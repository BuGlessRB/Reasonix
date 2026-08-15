package agent

import (
	"context"
	"fmt"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestGoalRunHasNoDefaultModelRoundCeiling(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	turns := make([]testutil.Turn, 0, 102)
	for i := range 101 {
		turns = append(turns, testutil.Turn{ToolCalls: []provider.ToolCall{{
			ID: fmt.Sprintf("r%d", i), Name: "read_file", Arguments: fmt.Sprintf(`{"path":"file-%d"}`, i),
		}}})
	}
	turns = append(turns, testutil.Turn{Text: "Done."})
	prov := testutil.NewMock("m", turns...)
	a := New(prov, reg, NewSession(""), Options{}, event.Discard)
	ctx := WithDeliveryExecutionScope(context.Background(), DeliveryExecutionScope{ID: "goal-1", TaskText: "work"})
	if err := a.Run(ctx, "work"); err != nil {
		t.Fatalf("Goal run stopped at a default round boundary: %v", err)
	}
	if prov.CallCount() != 102 {
		t.Fatalf("provider calls = %d, want 101 tool rounds plus final", prov.CallCount())
	}
}
func TestUnboundedGoalParentLeavesChildUnbounded(t *testing.T) {
	task := &TaskTool{}
	if got := task.childMaxStepsForContext(context.Background(), 0); got != 0 {
		t.Fatalf("child steps = %d, want unlimited", got)
	}
	if got := task.childMaxStepsForContext(context.Background(), 3); got != 3 {
		t.Fatalf("explicit child steps = %d, want 3", got)
	}
	task.maxSteps = 16
	if got := task.childMaxStepsForContext(context.Background(), 0); got != 8 {
		t.Fatalf("explicit parent child steps = %d, want 8", got)
	}
}
