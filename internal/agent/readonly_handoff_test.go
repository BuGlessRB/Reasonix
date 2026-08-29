package agent

import (
	"strings"
	"testing"

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
