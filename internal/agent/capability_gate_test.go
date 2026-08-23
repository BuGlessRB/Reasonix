package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/agentpreset"
	"reasonix/internal/evidence"
	"reasonix/internal/taskpolicy"
	"reasonix/internal/tool"
)

// Every production turn freezes a TaskPolicy before the first request, so the
// gate must hold with one installed — not only against the zero value the
// other cases in this file construct.
func TestDeliveryReviewGateHoldsUnderFrozenTaskPolicy(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.ReceiptFromToolCall("edit_file", json.RawMessage(`{"path":"internal/permission/gate.go"}`), true, evidence.ToolFacts{}))

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "review", readOnly: true})
	reg.Add(fakeTool{name: "security_review", readOnly: true})

	a := &Agent{deliveryProfile: true, task: taskRuntime{ledger: ledger}, svc: agentServices{tools: reg}}
	a.projectSensitivePaths = []string{"internal/permission/**"}
	a.turn.policy = taskpolicy.Derive(taskpolicy.Input{Preset: agentpreset.Delivery})
	a.turn.policySet = true

	if got := a.deliveryReviewGateFailure(); !strings.Contains(got, "high-risk") {
		t.Fatalf("gate under a frozen policy = %q, want high-risk review demand", got)
	}

	// Without the project's declaration the same edit is ordinary production
	// code: the host does not read sensitivity out of the path's spelling.
	a.projectSensitivePaths = nil
	if got := a.deliveryReviewGateFailure(); !strings.Contains(got, "medium-risk") {
		t.Fatalf("undeclared gate = %q, want medium-risk demand", got)
	}
}

func TestDeliveryReviewGateExplainsOpaqueMutationRecovery(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{
		ToolName:         "bash",
		Success:          true,
		Mutation:         true,
		MutationEvidence: evidence.MutationProven,
		Command:          "printf hi > out.log",
	})

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "review", readOnly: true})
	reg.Add(fakeTool{name: "security_review", readOnly: true})
	a := &Agent{deliveryProfile: true, task: taskRuntime{ledger: ledger}, svc: agentServices{tools: reg}}

	got := a.deliveryReviewGateFailure()
	for _, want := range []string{"high-risk", "reported no file paths", "reviewed_paths"} {
		if !strings.Contains(got, want) {
			t.Fatalf("review gate = %q, want %q", got, want)
		}
	}
	if strings.HasSuffix(got, "covering: ") {
		t.Fatalf("review gate must not end with empty coverage: %q", got)
	}

	ledger.Record(evidence.Receipt{ToolName: "review_report", Success: true, Args: json.RawMessage(`{
		"kind":"review",
		"verdict":"pass",
		"reviewed_paths":["internal/agent/agent.go"],
		"findings":[]
	}`)})
	got = a.deliveryReviewGateFailure()
	if !strings.Contains(got, "security_review") || !strings.Contains(got, "reported no file paths") {
		t.Fatalf("security review gate = %q, want opaque-mutation recovery guidance", got)
	}

	ledger.Record(evidence.Receipt{ToolName: "review_report", Success: true, Args: json.RawMessage(`{
		"kind":"security",
		"verdict":"pass",
		"reviewed_paths":["internal/agent/agent.go"],
		"findings":[]
	}`)})
	if got := a.deliveryReviewGateFailure(); got != "" {
		t.Fatalf("review gate = %q after both reports, want ready", got)
	}
}

func TestNonDeliveryProfileNeverRequiresStructuredReview(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.ReceiptFromToolCall("edit_file", json.RawMessage(`{"path":"internal/permission/gate.go"}`), true, evidence.ToolFacts{}))

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "review", readOnly: true})
	reg.Add(fakeTool{name: "security_review", readOnly: true})
	a := &Agent{deliveryProfile: false, task: taskRuntime{ledger: ledger}, svc: agentServices{tools: reg}}

	if got := a.deliveryReviewGateFailure(); got != "" {
		t.Fatalf("non-Delivery review gate = %q, want disabled", got)
	}
}

func TestDeliveryReviewGateHighRiskStillRequiresSecurityReview(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.ReceiptFromToolCall("edit_file", json.RawMessage(`{"path":"internal/permission/gate.go"}`), true, evidence.ToolFacts{}))

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "review", readOnly: true})
	reg.Add(fakeTool{name: "security_review", readOnly: true})
	a := &Agent{deliveryProfile: true, task: taskRuntime{ledger: ledger}, svc: agentServices{tools: reg}}
	a.projectSensitivePaths = []string{"internal/permission/**"}

	if got := a.deliveryReviewGateFailure(); !strings.Contains(got, "high-risk") {
		t.Fatalf("review gate = %q, want high-risk review demand", got)
	}

	ledger.Record(evidence.Receipt{ToolName: "review_report", Success: true, Args: json.RawMessage(`{
		"kind":"review",
		"verdict":"pass",
		"reviewed_paths":["internal/permission/gate.go"],
		"findings":[]
	}`)})
	if got := a.deliveryReviewGateFailure(); !strings.Contains(got, "security_review") {
		t.Fatalf("security review gate = %q, want security_review demand", got)
	}

	ledger.Record(evidence.Receipt{ToolName: "review_report", Success: true, Args: json.RawMessage(`{
		"kind":"security",
		"verdict":"pass",
		"reviewed_paths":["internal/permission/gate.go"],
		"findings":[]
	}`)})
	if got := a.deliveryReviewGateFailure(); got != "" {
		t.Fatalf("review gate = %q after both reports, want ready", got)
	}
}

func TestDeliveryReviewGateMediumAcceptsHostProvenVerificationAndCoverage(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.ReceiptFromToolCall("edit_file", json.RawMessage(`{"path":"internal/agent/parser.go"}`), true, evidence.ToolFacts{}))
	ledger.Record(evidence.ReceiptFromToolCall("bash", json.RawMessage(`{"command":"go test ./..."}`), true, evidence.ToolFacts{ReadOnly: true}))
	ledger.Record(evidence.ReceiptFromToolCall("read_file", json.RawMessage(`{"path":"internal/agent/parser.go"}`), true, evidence.ToolFacts{ReadOnly: true}))
	ledger.Record(evidence.Receipt{ToolName: "complete_step", Success: true, Args: json.RawMessage(`{
		"step":"fix parser",
		"evidence":[{"kind":"verification","command":"go test ./..."}]
	}`)})

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "review", readOnly: true})
	a := &Agent{deliveryProfile: true, task: taskRuntime{ledger: ledger}, svc: agentServices{tools: reg}}
	if got := a.deliveryReviewGateFailure(); got != "" {
		t.Fatalf("medium-risk host proof was rejected: %q", got)
	}

	missingVerification := evidence.NewLedger()
	missingVerification.Record(evidence.ReceiptFromToolCall("edit_file", json.RawMessage(`{"path":"internal/agent/parser.go"}`), true, evidence.ToolFacts{}))
	missingVerification.Record(evidence.ReceiptFromToolCall("read_file", json.RawMessage(`{"path":"internal/agent/parser.go"}`), true, evidence.ToolFacts{ReadOnly: true}))
	a.task.ledger = missingVerification
	if got := a.deliveryReviewGateFailure(); !strings.Contains(got, "host-proven verification") {
		t.Fatalf("medium-risk review without verification = %q, want host-proof guidance", got)
	}
}

func TestDeliveryReviewGateDefersToParentInSubagents(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.ReceiptFromToolCall("edit_file", json.RawMessage(`{"path":"internal/permission/gate.go"}`), true, evidence.ToolFacts{}))

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "review", readOnly: true})
	reg.Add(fakeTool{name: "security_review", readOnly: true})
	a := &Agent{agentConfig: agentConfig{subagentDepth: 1}, deliveryProfile: true, task: taskRuntime{ledger: ledger}, svc: agentServices{tools: reg}}

	// Inside a sub-agent the structured-review contract belongs to the parent,
	// which receives the child's mutation receipts via mergeChildEvidence. The
	// child must not wedge against a review_report demand it may be unable to
	// satisfy.
	if got := a.deliveryReviewGateFailure(); got != "" {
		t.Fatalf("subagent review gate = %q, want deferred to parent", got)
	}
}
