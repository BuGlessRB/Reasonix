package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// contractTool declares required fields and records whether it ever ran.
type contractTool struct{ ran *bool }

func (contractTool) Name() string        { return "remember" }
func (contractTool) Description() string { return "" }
func (contractTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"description":{"type":"string"},"body":{"type":"string"}},"required":["description","body"]}`)
}
func (contractTool) ReadOnly() bool { return false }
func (t contractTool) Execute(context.Context, json.RawMessage) (string, error) {
	*t.ran = true
	return "saved", nil
}

// An approval spent on a call that cannot execute is unrecoverable: the user
// answers the prompt, the tool then rejects its own arguments, and nothing was
// saved. The schema already said what the call must carry, so it is read first.
func TestMissingRequiredArgumentSkipsApproval(t *testing.T) {
	ran := false
	reg := tool.NewRegistry()
	reg.Add(contractTool{ran: &ran})
	g := &stubGate{}
	a := New(nil, reg, NewSession(""), Options{Gate: g}, event.Discard)

	out := a.executeOne(context.Background(), &a.turn, provider.ToolCall{
		Name: "remember", Arguments: `{"name":"pkg-manager","body":"the project uses pnpm"}`,
	})
	if !strings.Contains(out.output, `it requires "description"`) {
		t.Fatalf("refusal does not name the missing field: %q", out.output)
	}
	if len(g.checked) != 0 {
		t.Fatalf("permission was consulted for a call that cannot run: %v", g.checked)
	}
	if ran {
		t.Fatal("tool executed despite missing a required argument")
	}
}

// The check only removes calls the schema itself rejects: a complete one still
// reaches permission and the tool.
func TestCompleteArgumentsReachPermissionAndTool(t *testing.T) {
	ran := false
	reg := tool.NewRegistry()
	reg.Add(contractTool{ran: &ran})
	g := &stubGate{}
	a := New(nil, reg, NewSession(""), Options{Gate: g}, event.Discard)

	out := a.executeOne(context.Background(), &a.turn, provider.ToolCall{
		Name: "remember", Arguments: `{"description":"the project uses pnpm","body":"the project uses pnpm"}`,
	})
	if !ran || !strings.Contains(out.output, "saved") {
		t.Fatalf("well-formed call did not run: ran=%v out=%q", ran, out.output)
	}
	if len(g.checked) != 1 {
		t.Fatalf("permission consulted %d times, want 1 (%v)", len(g.checked), g.checked)
	}
}

// A tool call cut off mid-argument and one that mistyped an escape both arrive
// as "invalid JSON". They need opposite fixes, so the detail has to separate
// them: one run truncated a 2KB subtask summary and spent its retry hunting a
// syntax error that was never there.
func TestMalformedArgumentsSeparateTruncationFromTypos(t *testing.T) {
	cut := malformedArgumentsDetail(`{"status":"complete","summary":"Updated every fmt_amount(value`)
	if !strings.Contains(cut, "cut off") || !strings.Contains(cut, "short") {
		t.Errorf("truncated args detail = %q, want the length named as the cause", cut)
	}
	typo := malformedArgumentsDetail(`{"status":"complete","summary":"a \some path"}`)
	if !strings.Contains(typo, "byte 36") {
		t.Errorf("mistyped args detail = %q, want the offset named", typo)
	}
	if strings.Contains(typo, "cut off") {
		t.Errorf("mistyped args detail = %q, must not blame length", typo)
	}
}
