package agent

import (
	"context"
	"encoding/json"

	"reasonix/internal/safety/permission"
)

// verdictGate is a Gate that also names why it refused.
type verdictGate interface {
	Verdict(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (permission.Verdict, error)
}

// writerDenyingGate is a Gate whose posture refuses every call that is not a
// read, which a path around the gate has to honour as well.
type writerDenyingGate interface {
	DeniesWriters() bool
}

func gateVerdict(ctx context.Context, g Gate, toolName string, args json.RawMessage, readOnly bool) (permission.Verdict, error) {
	if vg, ok := g.(verdictGate); ok {
		return vg.Verdict(ctx, toolName, args, readOnly)
	}
	allow, reason, err := g.Check(ctx, toolName, args, readOnly)
	return permission.Verdict{Allow: allow, Reason: reason}, err
}

// readOnlyPostureBlock refuses a writer that skips the ordinary gate — an
// authorized MCP server — while the posture is read-only. Authorizing a server
// answered whether it may be called, not whether a read-only session may write.
func readOnlyPostureBlock(ctx context.Context, g Gate, toolName string, args json.RawMessage, readOnly bool) (toolOutcome, bool) {
	dg, ok := g.(writerDenyingGate)
	if readOnly || !ok || !dg.DeniesWriters() {
		return toolOutcome{}, false
	}
	v, err := gateVerdict(ctx, g, toolName, args, readOnly)
	if err == nil && v.Allow {
		return toolOutcome{}, false
	}
	return toolOutcome{output: "blocked: " + v.Reason, blocked: true, errMsg: "blocked by permission policy", refusalCode: v.Code}, true
}
