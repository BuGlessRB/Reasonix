package taskpolicy

import (
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/agentpreset"
)

func TestDeriveVerificationFollowsPreset(t *testing.T) {
	if p := Derive(Input{Preset: agentpreset.Balanced}); p.Verification != VerifyTargeted {
		t.Fatalf("balanced verification = %v, want targeted", p.Verification)
	}
	if p := Derive(Input{Preset: agentpreset.Delivery}); p.Verification != VerifyFull {
		t.Fatalf("delivery verification = %v, want full", p.Verification)
	}
}

// The host derives a turn's policy from structural signals only. Substring
// matching on user prose read "do not modify anything under tests/" as a
// blanket freeze and blocked the fix the same sentence asked for; limits the
// user wants belong to the permission system, which sees the real command.
func TestDeriveTakesNoUserProse(t *testing.T) {
	for _, f := range reflect.VisibleFields(reflect.TypeFor[Input]()) {
		switch f.Name {
		case "Preset", "PlanMode":
		default:
			t.Errorf("Input.%s: policy input must stay structural, never user text", f.Name)
		}
	}
}

func TestPlanModeForbidsMutation(t *testing.T) {
	p := Derive(Input{Preset: agentpreset.Delivery, PlanMode: true})
	if p.AllowsMutation() {
		t.Fatal("plan mode must forbid mutation")
	}
	if !Derive(Input{Preset: agentpreset.Delivery}).AllowsMutation() {
		t.Fatal("mutation must be allowed outside plan mode")
	}
}

func TestExecutionPolicyBlockStable(t *testing.T) {
	p := Derive(Input{Preset: agentpreset.Balanced})
	block := ExecutionPolicyBlock(p)
	if !strings.Contains(block, `preset="balanced"`) {
		t.Fatalf("block missing preset: %s", block)
	}
	if !strings.Contains(block, `version="3"`) {
		t.Fatalf("block missing version: %s", block)
	}
	if !strings.HasPrefix(block, "<execution-policy") || !strings.HasSuffix(block, "</execution-policy>") {
		t.Fatalf("bad block shape: %s", block)
	}
	if strings.Contains(block, "constraint=") {
		t.Fatalf("no constraint line belongs outside plan mode: %s", block)
	}
	if !strings.Contains(ExecutionPolicyBlock(Derive(Input{Preset: agentpreset.Balanced, PlanMode: true})), "constraint=plan-mode-read-only") {
		t.Fatal("plan mode must stay visible in the block")
	}
}
