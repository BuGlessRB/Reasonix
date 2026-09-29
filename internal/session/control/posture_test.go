package control

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/safety/permission"
	"reasonix/internal/state/sessionstore"
)

func TestDefaultApprovalModeNeedsConfinementAndTrust(t *testing.T) {
	for _, tc := range []struct {
		confined bool
		trust    config.WorkspaceTrust
		want     string
	}{
		{true, config.WorkspaceTrusted, ToolApprovalAuto},
		{true, config.WorkspaceTrustUndecided, ToolApprovalAsk},
		{true, config.WorkspaceTrustDeclined, ToolApprovalAsk},
		{false, config.WorkspaceTrusted, ToolApprovalAsk},
		{true, config.WorkspaceTrust("trusted "), ToolApprovalAsk},
	} {
		if got := DefaultApprovalMode(tc.confined, tc.trust); got != tc.want {
			t.Errorf("DefaultApprovalMode(%v, %q) = %q, want %q", tc.confined, tc.trust, got, tc.want)
		}
	}
}

func TestControllerReadsTrustFromItsOwnHome(t *testing.T) {
	home, ws := t.TempDir(), t.TempDir()
	c := New(Options{WorkspaceRoot: ws, Posture: PostureEvidence{WritesConfined: true, Home: home}})
	if got := c.DefaultApprovalMode(); got != ToolApprovalAsk {
		t.Fatalf("an undecided folder opens in %q", got)
	}
	if err := c.SetWorkspaceTrust(config.WorkspaceTrusted); err != nil {
		t.Fatal(err)
	}
	if got := c.DefaultApprovalMode(); got != ToolApprovalAuto {
		t.Fatalf("a trusted, confined folder opens in %q", got)
	}
	if other := New(Options{WorkspaceRoot: t.TempDir(), Posture: PostureEvidence{WritesConfined: true, Home: home}}); other.DefaultApprovalMode() != ToolApprovalAsk {
		t.Fatal("trust for one folder carried to another")
	}
	if homeless := New(Options{WorkspaceRoot: ws, Posture: PostureEvidence{WritesConfined: true}}); homeless.DefaultApprovalMode() != ToolApprovalAsk {
		t.Fatal("with no home to read trust from, the session must ask")
	}
}

// An interactive read-only session refuses a write outright: no prompt is
// raised, so no answer — and no session grant — can let it through.
func TestInteractiveReadOnlyRefusesWithoutAsking(t *testing.T) {
	writer := &recordingWriter{}
	reg := tool.NewRegistry()
	reg.Add(writer)
	prov := &scriptedTurns{turns: [][]provider.Chunk{toolCallTurn("c1", "write_file", `{"path":"a.txt"}`), textTurn("Done.")}}
	ag := agent.New(prov, reg, sessionstore.NewSession(""), agent.Options{}, event.Discard)
	prompts := 0
	c := New(Options{
		Runner: ag, Executor: ag,
		Policy: permission.New("ask", []string{"write_file"}, nil, nil),
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.ApprovalRequest {
				prompts++
			}
		}),
	})
	c.EnableInteractiveApproval()
	c.SetToolApprovalMode(ToolApprovalReadOnly)
	if err := c.runOneTurn(context.Background(), orchestratedTurn{input: "edit", raw: "edit"}); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if prompts != 0 || len(writer.paths) != 0 {
		t.Fatalf("read-only asked %d times and ran %v", prompts, writer.paths)
	}
}

// Headless read-only refuses even the create-only memory a headless session
// may otherwise write, and every sub-agent gate shares the posture.
func TestHeadlessReadOnlyRefusesEveryWriter(t *testing.T) {
	gate := NewSharedHeadlessGate(permission.New("ask", []string{"write_file"}, nil, nil), ToolApprovalAuto)
	gate.Update(ToolApprovalReadOnly)
	inner := gate.current()
	inner.allowLowRiskFreshAction = func(string, json.RawMessage) bool { return true }
	ctx := context.Background()
	for _, name := range []string{"write_file", memoryRememberTool} {
		v, err := gate.Verdict(ctx, name, json.RawMessage(`{"path":"a"}`), false)
		if err != nil || v.Allow || v.Code != permission.RefusalReadOnly {
			t.Fatalf("%s: %+v %v, want a read-only refusal", name, v, err)
		}
	}
	if !gate.DeniesWriters() {
		t.Fatal("the shared gate must report its read-only posture")
	}
	if v, _ := gate.Verdict(ctx, "read_file", json.RawMessage(`{"path":"a"}`), true); !v.Allow {
		t.Fatalf("a read was refused: %+v", v)
	}
	gate.Update(ToolApprovalAsk)
	if gate.DeniesWriters() {
		t.Fatal("leaving read-only must lift it")
	}
	if v, _ := gate.Verdict(ctx, "write_file", json.RawMessage(`{"path":"a"}`), false); !v.Allow {
		t.Fatalf("an allow rule stands again once read-only is left: %+v", v)
	}
}

func TestHeadlessRefusalsNameNobodyAsTheCause(t *testing.T) {
	gate := BuildHeadlessApprovalGate(permission.New("ask", nil, nil, nil), ToolApprovalAsk)
	v, _ := gate.Verdict(context.Background(), "write_file", json.RawMessage(`{"path":"a"}`), false)
	if v.Allow || v.Code != permission.RefusalUnattended {
		t.Fatalf("an unattended ask is %+v", v)
	}
	v, _ = gate.Verdict(context.Background(), memoryForgetTool, json.RawMessage(`{"name":"a"}`), false)
	if v.Allow || v.Code != permission.RefusalUnattended {
		t.Fatalf("a fresh human decision with nobody there is %+v", v)
	}
}
