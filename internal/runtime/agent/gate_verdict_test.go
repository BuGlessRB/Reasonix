package agent

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/safety/permission"
)

type checkOnlyGate struct{}

func (checkOnlyGate) Check(context.Context, string, json.RawMessage, bool) (bool, string, error) {
	return false, "no", nil
}

// An authorized MCP server skips the ordinary gate; a read-only posture still
// refuses its writers, and lets its readers and every other posture through.
func TestReadOnlyPostureBlocksWritersThatSkipTheGate(t *testing.T) {
	ctx := context.Background()
	policy := permission.New("allow", []string{"mcp__srv__put"}, nil, nil)
	policy.ReadOnly = true
	readOnly := permission.NewGate(policy, nil)
	out, blocked := readOnlyPostureBlock(ctx, readOnly, "mcp__srv__put", json.RawMessage(`{}`), false)
	if !blocked || out.refusalCode != permission.RefusalReadOnly || !out.blocked {
		t.Fatalf("a writer through the MCP fast path ran in read-only mode: %+v", out)
	}
	if _, blocked := readOnlyPostureBlock(ctx, readOnly, "mcp__srv__get", json.RawMessage(`{}`), true); blocked {
		t.Fatal("a declared MCP reader was refused")
	}
	open := permission.NewGate(permission.New("ask", nil, nil, nil), nil)
	if _, blocked := readOnlyPostureBlock(ctx, open, "mcp__srv__put", json.RawMessage(`{}`), false); blocked {
		t.Fatal("outside read-only the fast path is left to the deny list")
	}
	if _, blocked := readOnlyPostureBlock(ctx, checkOnlyGate{}, "mcp__srv__put", json.RawMessage(`{}`), false); blocked {
		t.Fatal("a gate with no posture to report is not read-only")
	}
	if v, _ := gateVerdict(ctx, checkOnlyGate{}, "x", nil, false); v.Allow || v.Reason != "no" || v.Code != "" {
		t.Fatalf("a Check-only gate keeps its answer and names no code: %+v", v)
	}
}
