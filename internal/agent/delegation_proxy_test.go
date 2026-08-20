package agent

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/capability"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/plugin"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// A capability-proxied call must still produce the delegation receipt: the
// proxy resolves the registry tool and calls it with the same context, so a
// wrapper that dropped the optional audit channel here would leave write-claim
// violations and criterion downgrades unrecorded, not just a counter at zero.
type proxyProbe struct {
	event.Sink
	got []evidence.DelegationAudit
}

func (p *proxyProbe) RecordDelegationAudit(a evidence.DelegationAudit) { p.got = append(p.got, a) }

func TestDelegationAuditSurvivesCapabilityProxy(t *testing.T) {
	probe := &proxyProbe{Sink: event.Discard}
	reg := tool.NewRegistry()
	reg.Add(fakeReadFileTool{})
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{toolCallChunk("1", "read_file", `{"path":"src/parser.go"}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
	}}
	task := NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0.0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(mustSubagentStore(t), t.TempDir(), "base", "high").
		WithScheduler(NewSubagentScheduler(4, 4))
	reg.Add(task)

	ctx := context.Background()
	runtime := NewMCPCapabilityRuntime(ctx, plugin.NewHost(), []plugin.Spec{}, reg, nil)
	proxy := runtime.NewFrontend(capability.NewLedger(), nil)

	args, err := json.Marshal(map[string]any{
		"action":        "call",
		"capability_id": "tool:" + task.Name(),
		"arguments":     map[string]string{"prompt": "read src/parser.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cctx := withCallContext(ctx, "call-1", probe, nil, false)
	if _, err := proxy.Execute(cctx, args); err != nil {
		t.Fatalf("use_capability -> task: %v", err)
	}
	if len(probe.got) != 1 {
		t.Fatalf("got %d delegation audits through the capability proxy, want 1", len(probe.got))
	}
}
