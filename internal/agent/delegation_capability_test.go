package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/capability"
	"reasonix/internal/tool"
)

// TestDelegationSurvivesTheProxyBoundary is the gate the frontend depended on
// and never had. The model only ever calls use_capability, so a dispatch that
// loses its profile there reaches the UI anonymous: the panel counted no
// sub-agents at all, and the card fell back to reading a delegate's step count
// as a number of delegates.
func TestDelegationSurvivesTheProxyBoundary(t *testing.T) {
	reg := tool.NewRegistry()
	task := &TaskTool{}
	reg.Add(task)
	reg.Add(NewFleetTool(task))
	reg.Add(NewReadOnlyTaskTool(task))
	catalog := func() capability.Catalog {
		return capability.BuildCatalog(capability.CatalogOptions{Tools: reg.AllContractEntries()})
	}
	uc := NewUseCapabilityTool(context.Background(), nil, nil, reg, capability.NewLedger(), nil, catalog)

	for _, tc := range []struct {
		id, args  string
		wantName  string
		wantCount int
	}{
		{`task:subagent`, `{"prompt":"x"}`, "task", 1},
		{`task:subagent`, `{"prompt":"x","profile":"implementer"}`, "implementer", 1},
		{`task:fleet`, `{"tasks":[{"prompt":"a"},{"prompt":"b"},{"prompt":"c"}]}`, "fleet", 3},
	} {
		call := `{"action":"call","capability_id":"` + tc.id + `","arguments":` + tc.args + `}`
		rc, err := uc.ResolveCall(context.Background(), json.RawMessage(call))
		if err != nil {
			t.Fatalf("resolve %s: %v", tc.id, err)
		}
		got := delegationProfile(rc.Target, rc.Args)
		if got == nil {
			t.Errorf("%s %s resolved to a dispatch with no profile — the UI cannot name or count it", tc.id, tc.args)
			continue
		}
		if got.Name != tc.wantName || got.Count != tc.wantCount {
			t.Errorf("%s %s reported %q×%d, want %q×%d", tc.id, tc.args, got.Name, got.Count, tc.wantName, tc.wantCount)
		}
	}
}

// TestNonDelegatingCallHasNoProfile keeps the mark meaningful: the frontend
// reads a profile's presence as "this work left the context", so an ordinary
// tool must not carry one.
func TestNonDelegatingCallHasNoProfile(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(probeInertTool{})
	catalog := func() capability.Catalog {
		return capability.BuildCatalog(capability.CatalogOptions{Tools: reg.AllContractEntries()})
	}
	uc := NewUseCapabilityTool(context.Background(), nil, nil, reg, capability.NewLedger(), nil, catalog)
	rc, err := uc.ResolveCall(context.Background(), json.RawMessage(`{"action":"call","capability_id":"tool:inert","arguments":{}}`))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := delegationProfile(rc.Target, rc.Args); got != nil {
		t.Errorf("an ordinary tool reported a delegation: %+v", got)
	}
	if got := delegationProfile(nil, nil); got != nil {
		t.Errorf("an unresolved target reported a delegation: %+v", got)
	}
}

// TestDelegationCountIsAgentsNotSteps pins the distinction the UI got wrong: a
// batch dispatcher reports how many contexts it opened, and a malformed or
// empty batch reports none rather than inventing one.
func TestDelegationCountIsAgentsNotSteps(t *testing.T) {
	for _, tc := range []struct {
		args string
		want int
	}{
		{`{"tasks":[{"prompt":"a"},{"prompt":"b"}]}`, 2},
		{`{"tasks":[]}`, 0},
		{`{`, 0},
	} {
		got := batchDelegation("fleet", json.RawMessage(tc.args))
		if got == nil || got.Count != tc.want {
			t.Errorf("fleet %s reported %v, want count %d", tc.args, got, tc.want)
		}
		if got != nil && !strings.EqualFold(got.Name, "fleet") {
			t.Errorf("fleet dispatch named %q", got.Name)
		}
	}
}

type probeInertTool struct{}

func (probeInertTool) Name() string            { return "inert" }
func (probeInertTool) Description() string     { return "does not delegate" }
func (probeInertTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (probeInertTool) ReadOnly() bool          { return true }
func (probeInertTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}
