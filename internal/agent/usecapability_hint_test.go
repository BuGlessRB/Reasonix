package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/tool"
)

// The fleet call that motivated this: "items" was rejected without the error
// naming "tasks", and finding that one name cost an inspect and two doc calls.
func TestContractHintNamesTheParameterTheCallShouldHaveUsed(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"tasks":{},"max_parallel":{}},"required":["tasks"]}`)
	got := contractHint(schema, json.RawMessage(`{"items":[]}`))
	for _, want := range []string{`"items" is not a parameter`, `requires "tasks"`, `accepts "max_parallel", "tasks"`} {
		if !strings.Contains(got, want) {
			t.Errorf("hint = %q, want it to contain %q", got, want)
		}
	}
}

// A hint on a failure it cannot explain is noise: the capability may have
// failed for its own reasons while the arguments fit the schema exactly.
func TestContractHintStaysSilentWhenTheArgumentsFit(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"tasks":{}},"required":["tasks"]}`)
	if got := contractHint(schema, json.RawMessage(`{"tasks":[]}`)); got != "" {
		t.Errorf("arguments that fit produced hint %q, want silence", got)
	}
	// Absent arguments do break a contract that requires one, so that case is
	// deliberately not silent.
	if got := contractHint(schema, json.RawMessage(``)); !strings.Contains(got, `requires "tasks"`) {
		t.Errorf("missing required parameter went unreported: %q", got)
	}
	if got := contractHint(json.RawMessage(``), json.RawMessage(`{"items":[]}`)); got != "" {
		t.Errorf("no schema must produce no hint, got %q", got)
	}
	if got := contractHint(json.RawMessage(`{"type":"object"}`), json.RawMessage(`{"items":[]}`)); got != "" {
		t.Errorf("a schema without properties must produce no hint, got %q", got)
	}
}

// The delegation calls that motivated this: a fleet item was refused with
// `json: unknown field "name"`, which names neither the level the field sits on
// nor what that level accepts. One observed run tried "name", then "title",
// then "description", paying a round trip for each before finding "prompt".
func TestContractHintDescendsIntoArrayItems(t *testing.T) {
	schema := (&FleetTool{}).Schema()
	got := contractHint(schema, json.RawMessage(`{"tasks":[{"name":"fix-alpha","prompt":"do it"}]}`))
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

// An item that omits what its own schema requires is answered at the item's
// level too, so the fix does not read as a missing top-level parameter. The
// fixture is inline because a fleet item requires prompt or adopt_ref — an
// alternation no single `required` entry states, so fleet declares neither and
// refuses the pair at the host (validateFleetItemShape).
func TestContractHintNamesWhatAnItemRequires(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"prompt":{},"description":{}},"required":["prompt"]}}},"required":["tasks"]}`)
	got := contractHint(schema, json.RawMessage(`{"tasks":[{"description":"fix billing"}]}`))
	if !strings.Contains(got, "`tasks`"+` item requires "prompt"`) {
		t.Errorf("hint = %q, want the item's own required field named", got)
	}
}

// Well-formed items stay silent: descending must not invent a contract breach
// where the call matches the schema at every level.
func TestContractHintSilentOnWellFormedItems(t *testing.T) {
	schema := (&FleetTool{}).Schema()
	if got := contractHint(schema, json.RawMessage(`{"tasks":[{"prompt":"a"},{"prompt":"b","model":"m"}]}`)); got != "" {
		t.Errorf("well-formed items produced hint %q, want silence", got)
	}
}

// The end of the path the hint exists for: a capability call whose target
// refused the arguments comes back carrying the level the field belongs to.
func TestCapabilityCallFailureCarriesTheItemContract(t *testing.T) {
	uc := &UseCapabilityTool{}
	resolved := tool.ResolvedCall{
		Target: &FleetTool{},
		Args:   json.RawMessage(`{"tasks":[{"name":"fix-alpha","prompt":"do it"}]}`),
	}
	err := uc.recordCallFailure(resolved, errors.New(`invalid args: json: unknown field "name"`))
	if err == nil || !strings.Contains(err.Error(), "`tasks`"+" item") {
		t.Fatalf("err = %v, want the item level named alongside the target's own message", err)
	}
}
