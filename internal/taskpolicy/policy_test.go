package taskpolicy

import (
	"strings"
	"testing"

	"reasonix/internal/agentpreset"
)

func TestDeriveVerificationFollowsPreset(t *testing.T) {
	if p := Derive(Input{Raw: "fix it", Preset: agentpreset.Balanced}); p.Verification != VerifyTargeted {
		t.Fatalf("balanced verification = %v, want targeted", p.Verification)
	}
	if p := Derive(Input{Raw: "fix it", Preset: agentpreset.Delivery}); p.Verification != VerifyFull {
		t.Fatalf("delivery verification = %v, want full", p.Verification)
	}
}

// The turn's obligations come from the role setting and from what the user
// said outright. Nothing about the topic or shape of the message may add one:
// a sentence that merely mentions auth or lists three steps is still one turn
// under the same preset.
func TestDeriveReadsNothingOffTheTopic(t *testing.T) {
	base := Derive(Input{Raw: "hi", Preset: agentpreset.Balanced})
	for _, raw := range []string{
		"fix the authentication bypass in production login",
		"migrate the schema and deploy it to prod",
		"1. read the config\n2. rewrite the loader\n3. run the tests",
		"迁移这段配置到新格式并发布",
	} {
		got := Derive(Input{Raw: raw, Preset: agentpreset.Balanced})
		if got.Verification != base.Verification {
			t.Errorf("%q derived verification=%v from words alone", raw, got.Verification)
		}
		if ExecutionPolicyBlock(got) != ExecutionPolicyBlock(base) {
			t.Errorf("%q produced a different policy block from words alone", raw)
		}
	}
}

func TestConstraintsNoMutation(t *testing.T) {
	p := Derive(Input{
		Raw:    "只分析这段代码的问题，不要修改",
		Preset: agentpreset.Balanced,
	})
	if p.AllowsMutation() {
		t.Fatal("must forbid mutation")
	}
}

func TestConstraintsNoTests(t *testing.T) {
	p := Derive(Input{
		Raw:    "fix the bug but don't run tests",
		Preset: agentpreset.Balanced,
	})
	if p.AllowsTests() {
		t.Fatal("must forbid tests")
	}
}

func TestConstraintsOnlyRun(t *testing.T) {
	p := Derive(Input{
		Raw:    "fix the parser, only run go test ./internal/parser",
		Preset: agentpreset.Balanced,
	})
	if !p.AllowsCommand("go test ./internal/parser") {
		t.Fatal("allowed check should pass")
	}
	if p.AllowsCommand("npm test") {
		t.Fatal("other checks should be blocked")
	}
	if p.AllowsCommand("go test ./internal/parser && npm test") {
		t.Fatal("a second shell command must not inherit the go test allowance")
	}
}

func TestConstraintsNoPush(t *testing.T) {
	p := Derive(Input{
		Raw:    "fix the bug, don't push",
		Preset: agentpreset.Balanced,
	})
	if !p.AllowsMutation() {
		t.Fatal("local mutation should still be allowed")
	}
	if p.AllowsExternal() {
		t.Fatal("push must be forbidden")
	}
}

func TestQuotedConstraintsIgnored(t *testing.T) {
	raw := "please fix the bug\n```\ndon't modify anything\n```\n"
	p := Derive(Input{
		Raw:         raw,
		Instruction: StripQuotedConstraints(raw),
		Preset:      agentpreset.Balanced,
	})
	if !p.AllowsMutation() {
		t.Fatal("quoted no-modify must not bind the host")
	}
}

func TestPlanModeForbidsMutation(t *testing.T) {
	p := Derive(Input{
		Raw:      "implement the feature",
		Preset:   agentpreset.Delivery,
		PlanMode: true,
	})
	if p.AllowsMutation() {
		t.Fatal("plan mode must forbid mutation")
	}
}

func TestExecutionPolicyBlockStable(t *testing.T) {
	p := Derive(Input{Raw: "fix it", Preset: agentpreset.Balanced})
	block := ExecutionPolicyBlock(p)
	if !strings.Contains(block, `preset="balanced"`) {
		t.Fatalf("block missing preset: %s", block)
	}
	if !strings.Contains(block, `version="2"`) {
		t.Fatalf("block missing version: %s", block)
	}
	if !strings.HasPrefix(block, "<execution-policy") || !strings.HasSuffix(block, "</execution-policy>") {
		t.Fatalf("bad block shape: %s", block)
	}
}
