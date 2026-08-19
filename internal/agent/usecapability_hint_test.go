package agent

import (
	"encoding/json"
	"strings"
	"testing"
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
