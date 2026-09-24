package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/planmode"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/runtime/capability"
	"reasonix/internal/runtime/delegation"
	"reasonix/internal/state/sessionstore"
	"strings"
	"testing"
)

// Delegation spawns work rather than changing state, so a failed one does not
// open the dependency barrier — batchCallIsMutatingFailure exempts it. Being
// skipped BY the barrier is the same question, and a bare !ReadOnly answers it
// the other way, dropping every delegation left in the batch.
func TestBatchDoesNotSkipDelegationAsADependentModification(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(&delegation.TaskTool{})

	for _, name := range []string{"task"} {
		call := provider.ToolCall{Name: name, Arguments: `{"prompt":"do a thing","description":"d"}`}
		if agent.BatchCallStaticallySkippable(reg, call) {
			t.Errorf("%s is skippable as a dependent modification, but a failed %s does not open the barrier either",
				name, name)
		}
	}
}

// TestDelegationSurvivesTheProxyBoundary is the gate the frontend depended on
// and never had. The model only ever calls use_capability, so a dispatch that
// loses its profile there reaches the UI anonymous: the panel counted no
// sub-agents at all, and the card fell back to reading a delegate's step count
// as a number of delegates.
func TestDelegationSurvivesTheProxyBoundary(t *testing.T) {
	reg := tool.NewRegistry()
	task := &delegation.TaskTool{}
	reg.Add(task)
	reg.Add(delegation.NewFleetTool(task))
	reg.Add(delegation.NewReadOnlyTaskTool(task))
	catalog := func() capability.Catalog {
		return capability.BuildCatalog(capability.CatalogOptions{Tools: reg.AllContractEntries()})
	}
	uc := agent.NewUseCapabilityTool(context.Background(), nil, nil, reg, capability.NewLedger(), nil, catalog)

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
		got := agent.DelegationProfile(rc.Target, rc.Args)
		if got == nil {
			t.Errorf("%s %s resolved to a dispatch with no profile — the UI cannot name or count it", tc.id, tc.args)
			continue
		}
		if got.Name != tc.wantName || got.Count != tc.wantCount {
			t.Errorf("%s %s reported %q×%d, want %q×%d", tc.id, tc.args, got.Name, got.Count, tc.wantName, tc.wantCount)
		}
	}
}

// Planning keeps the delegation it was designed around. read_only_task exists
// so a plan can research in an isolated context; a barrier reading "delegation"
// as "side effect" would take that away, and one reading "no mutation receipt"
// as "safe" lets the writer-capable ones through — the hole this closes.
func TestPlanningPhaseSplitsDelegationByWriterCapability(t *testing.T) {
	task := &delegation.TaskTool{}
	reg := tool.NewRegistry()
	a := agent.New(nil, reg, sessionstore.NewSession(""), agent.Options{}, event.Discard)
	a.SetPlanMode(true)

	blocked := map[string]bool{}
	for _, tl := range []tool.Tool{
		task,
		delegation.NewReadOnlyTaskTool(task),
		delegation.NewFleetTool(task),
		delegation.NewParallelTasksTool(task, reg),
		delegation.NewSubagentResultTool(task),
		delegation.NewSubagentListTool(task),
	} {
		safety := planmode.PlanSafetyUnknown
		if c, ok := tl.(tool.PlanModeClassifier); ok {
			safety = planmode.PlanSafetyUnsafe
			if c.PlanModeSafe() {
				safety = planmode.PlanSafetySafe
			}
		}
		got := agent.PlanModeDecision(a, tl, safety)
		blocked[tl.Name()] = got.Blocked
		// The verdict has to follow what the tool declares about itself, or the
		// barrier has started classifying delegation by name.
		want := !tl.ReadOnly() && safety != planmode.PlanSafetySafe
		if got.Blocked != want {
			t.Errorf("%q readOnly=%v safety=%v blocked=%v, want %v", tl.Name(), tl.ReadOnly(), safety, got.Blocked, want)
		}
	}

	for name, wantBlocked := range map[string]bool{
		"read_only_task":       false,
		"parallel_tasks":       false,
		"read_subagent_result": false,
		"list_subagents":       false,
		"task":                 true,
	} {
		got, ok := blocked[name]
		if !ok {
			t.Errorf("%q was not registered; the anchor no longer measures anything", name)
			continue
		}
		if got != wantBlocked {
			t.Errorf("%q blocked=%v during planning, want %v", name, got, wantBlocked)
		}
	}
}

// The delegation calls that motivated this: a fleet item was refused with
// `json: unknown field "name"`, which names neither the level the field sits on
// nor what that level accepts. One observed run tried "name", then "title",
// then "description", paying a round trip for each before finding "prompt".
func TestContractHintDescendsIntoArrayItems(t *testing.T) {
	schema := (&delegation.FleetTool{}).Schema()
	got := agent.ContractHint(schema, json.RawMessage(`{"tasks":[{"name":"fix-alpha","prompt":"do it"}]}`))
	// Membership, not position: the accepted list is alphabetical, so anchoring
	// on whichever key sorts first breaks every time one is added.
	for _, want := range []string{`"name" is not a parameter of a ` + "`tasks`" + ` item`, ` accepts `, `"depends_on"`, `"prompt"`} {
		if !strings.Contains(got, want) {
			t.Errorf("hint = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "this capability") {
		t.Errorf("hint = %q, want the item level named, not the outer call", got)
	}
}

// Well-formed items stay silent: descending must not invent a contract breach
// where the call matches the schema at every level.
func TestContractHintSilentOnWellFormedItems(t *testing.T) {
	schema := (&delegation.FleetTool{}).Schema()
	if got := agent.ContractHint(schema, json.RawMessage(`{"tasks":[{"prompt":"a"},{"prompt":"b","model":"m"}]}`)); got != "" {
		t.Errorf("well-formed items produced hint %q, want silence", got)
	}
}

// The end of the path the hint exists for: a capability call whose target
// refused the arguments comes back carrying the level the field belongs to.
func TestCapabilityCallFailureCarriesTheItemContract(t *testing.T) {
	uc := &agent.UseCapabilityTool{}
	resolved := tool.ResolvedCall{
		Target: &delegation.FleetTool{},
		Args:   json.RawMessage(`{"tasks":[{"name":"fix-alpha","prompt":"do it"}]}`),
	}
	err := agent.RecordCallFailure(uc, resolved, errors.New(`invalid args: json: unknown field "name"`))
	if err == nil || !strings.Contains(err.Error(), "`tasks`"+" item") {
		t.Fatalf("err = %v, want the item level named alongside the target's own message", err)
	}
}
