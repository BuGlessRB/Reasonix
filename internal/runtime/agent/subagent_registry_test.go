package agent

import (
	"context"
	"encoding/json"
	"reasonix/internal/runtime/capability"
	"strings"
	"testing"

	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
)

func TestFilterCapabilityListResultFailClosed(t *testing.T) {
	// Empty server set must not return the raw full inventory.
	full := `{"servers":[{"name":"secret-db","capability_id":"mcp-server:secret-db","status":"configured","authorized":true,"connected":false}],"note":"all"}`
	out := filterCapabilityListResult(full, nil)
	if strings.Contains(out, "secret-db") {
		t.Fatalf("empty servers must fail closed:\n%s", out)
	}
	if !strings.Contains(out, `"servers": []`) && !strings.Contains(out, `"servers":[]`) {
		t.Fatalf("expected empty servers array:\n%s", out)
	}

	// Malformed JSON must not pass through raw text that might contain names.
	leaky := `not-json but mentions secret-db and production`
	out = filterCapabilityListResult(leaky, map[string]bool{"alpha": true})
	if strings.Contains(out, "secret-db") || strings.Contains(out, "not-json") {
		t.Fatalf("malformed payload must fail closed:\n%s", out)
	}
	if !strings.Contains(out, `"servers"`) {
		t.Fatalf("fail-closed payload should still be JSON list shape:\n%s", out)
	}
}

func TestRestrictedListWithEmptyServersMapFailClosed(t *testing.T) {
	// Direct unit path: restricted proxy with empty servers still filters list.
	inner := NewUseCapabilityTool(context.Background(), nil, []plugin.Spec{
		{Name: "secret-db", Authorized: true},
	}, tool.NewRegistry(), nil, nil, nil)
	proxy := &restrictedCapabilityProxy{
		Tool:     inner,
		resolver: inner,
		allowed:  map[string]bool{"mcp-tool:incomplete": true}, // invalid shape should never happen after validation
		servers:  map[string]bool{},
	}
	rc, err := proxy.ResolveCall(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rc.Result, "secret-db") {
		t.Fatalf("empty servers map must not leak inventory:\n%s", rc.Result)
	}
}

func TestPlannerToolRegistryClonesUseCapability(t *testing.T) {
	parent := tool.NewRegistry()
	ledger := capability.NewLedger()
	proxy := NewUseCapabilityTool(context.Background(), nil, nil, parent, ledger, nil, nil)
	parent.Add(proxy)
	parent.Add(subagentRegistryTool{name: "read_file", readOnly: true})

	planner := PlannerToolRegistry(parent)
	got, ok := planner.Get("use_capability")
	if !ok {
		t.Fatal("planner missing use_capability")
	}
	uc, ok := got.(*UseCapabilityTool)
	if !ok {
		t.Fatalf("planner proxy type = %T, want *UseCapabilityTool", got)
	}
	if uc == proxy {
		t.Fatal("planner must not share the executor UseCapabilityTool pointer")
	}
	if uc.ledger == ledger {
		t.Fatal("planner frontend must not share the executor capability ledger")
	}
}
