package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/instruction"
	"reasonix/internal/provider"
	"reasonix/internal/taskpolicy"
	"reasonix/internal/tool"
)

// A registered explore worker is the model's to call. The turn policy carries
// the plan-mode boundary, not a quota on how the model gathers context.
func TestTaskPolicyLeavesExploreSubagentAlone(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "explore", readOnly: true})
	a := New(&scriptedProvider{name: "p"}, reg, NewSession("sys"), Options{}, event.Discard)
	a.turn.policy = taskpolicy.Derive(taskpolicy.Input{})
	a.turn.policySet = true

	got := a.executeOne(context.Background(), &a.turn, provider.ToolCall{Name: "explore", Arguments: `{}`})
	if got.blocked {
		t.Fatalf("explore outcome = %+v, want no policy block", got)
	}
}

// Ordinary work carries no host-invented limits: the gate blocks a writer only
// under plan mode, and reaches for no reading of what the user wrote.
func TestTaskPolicyBlocksWritersOnlyInPlanMode(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false})
	a := New(&scriptedProvider{name: "p"}, reg, NewSession("sys"), Options{}, event.Discard)
	a.turn.policySet = true
	call := provider.ToolCall{Name: "write_file", Arguments: `{"path":"notes.txt","content":"x"}`}

	a.turn.policy = taskpolicy.Derive(taskpolicy.Input{})
	if got := a.executeOne(context.Background(), &a.turn, call); got.blocked {
		t.Fatalf("outcome = %+v, want no policy block outside plan mode", got)
	}
	a.turn.policy = taskpolicy.Derive(taskpolicy.Input{PlanMode: true})
	got := a.executeOne(context.Background(), &a.turn, call)
	if !got.blocked || !strings.Contains(got.errMsg, "forbids mutation") {
		t.Fatalf("plan-mode outcome = %+v, want mutation block", got)
	}
}

func TestTaskPolicyRequiresPostMutationVerification(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash", readOnly: true})
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Mutation: true}
	check := evidence.Receipt{ToolName: "bash", Success: true, Command: "go test ./..."}
	a := &Agent{
		task: taskRuntime{ledger: readinessLedger(check, writer)},
		svc:  agentServices{tools: reg},
		turn: turnRuntime{
			policy:    taskpolicy.TaskPolicy{Verification: taskpolicy.VerifyTargeted},
			policySet: true,
		},
	}
	if got := a.finalReadinessCheckFor(); !strings.Contains(got.reason, "verification command") {
		t.Fatalf("readiness = %+v, want post-mutation verification", got)
	}
	a.task.ledger.Record(check)
	if got := a.finalReadinessCheckFor(); got.reason != "" {
		t.Fatalf("readiness after verification = %+v, want ready", got)
	}
}

// A project that declares its own checks has defined what verification means
// there. Running them satisfies the floor even when the host's classifier does
// not recognize the command — it cannot tell a test script from a deploy one.
func TestDeclaredProjectChecksSatisfyVerificationFloor(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash", readOnly: true})
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Mutation: true}
	unrecognized := evidence.Receipt{ToolName: "bash", Success: true, Command: "python3 test_stats.py"}
	newAgent := func(checks []instruction.VerifyCheck, receipts ...evidence.Receipt) *Agent {
		return &Agent{
			task:          taskRuntime{ledger: readinessLedger(receipts...)},
			svc:           agentServices{tools: reg},
			projectChecks: checks,
			turn: turnRuntime{
				policy:    taskpolicy.TaskPolicy{Verification: taskpolicy.VerifyTargeted},
				policySet: true,
			},
		}
	}
	declared := []instruction.VerifyCheck{{Command: "python3 test_stats.py", SourcePath: "REASONIX.md"}}

	if got := newAgent(nil, writer, unrecognized).finalReadinessCheckFor(); !strings.Contains(got.reason, "verification command") {
		t.Fatalf("undeclared project: readiness = %+v, want the classifier floor to still apply", got)
	}
	if got := newAgent(declared, writer, unrecognized).finalReadinessCheckFor(); got.reason != "" {
		t.Fatalf("declared check ran: readiness = %+v, want ready", got)
	}
	if got := newAgent(declared, writer).finalReadinessCheckFor(); !strings.Contains(got.reason, "test_stats.py") {
		t.Fatalf("declared check skipped: readiness = %+v, want it demanded", got)
	}
}
