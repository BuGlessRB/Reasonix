package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"reasonix/internal/tool"
)

// misplacedArgumentHint reports that a required top-level field was supplied one
// level too deep, inside "arguments". Answering "field is required" to a caller
// that did supply it starts a retry loop: the value looks present, so the next
// attempt only rearranges the nesting.
func misplacedArgumentHint(arguments json.RawMessage, field string) string {
	var nested map[string]json.RawMessage
	if len(arguments) == 0 || json.Unmarshal(arguments, &nested) != nil {
		return ""
	}
	if _, ok := nested[field]; !ok {
		return ""
	}
	return fmt.Sprintf(" — it is nested inside \"arguments\"; move %q up beside \"action\" and leave only the capability's own arguments inside", field)
}

// contractHint names the parameters a capability accepts, but only when the
// call actually broke that contract. A rejected call otherwise reports the
// violation without the contract — "unknown field \"items\"" does not say the
// field is called "tasks" — and the model spends a round trip on inspect and
// two more in the docs to learn one name.
func contractHint(schema, arguments json.RawMessage) string {
	var contract struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if len(schema) == 0 || json.Unmarshal(schema, &contract) != nil || len(contract.Properties) == 0 {
		return ""
	}
	var got map[string]json.RawMessage
	if len(arguments) > 0 && json.Unmarshal(arguments, &got) != nil {
		return ""
	}
	var unknown, missing []string
	for field := range got {
		if _, ok := contract.Properties[field]; !ok {
			unknown = append(unknown, field)
		}
	}
	for _, field := range contract.Required {
		if _, ok := got[field]; !ok {
			missing = append(missing, field)
		}
	}
	// Staying silent when the arguments fit the schema keeps the hint out of
	// failures it cannot explain, such as one the capability itself reported.
	if len(unknown) == 0 && len(missing) == 0 {
		return ""
	}
	slices.Sort(unknown)
	slices.Sort(missing)
	var b strings.Builder
	if len(unknown) > 0 {
		fmt.Fprintf(&b, "; %s is not a parameter of this capability", strings.Join(quoted(unknown), ", "))
	}
	if len(missing) > 0 {
		fmt.Fprintf(&b, "; it requires %s", strings.Join(quoted(missing), ", "))
	}
	fmt.Fprintf(&b, "; it accepts %s", strings.Join(quoted(slices.Sorted(maps.Keys(contract.Properties))), ", "))
	return b.String()
}

func quoted(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = fmt.Sprintf("%q", name)
	}
	return out
}

// recordCallFailure books a failed capability call and returns what the model
// reads, carrying the contract when the arguments were what broke it. The
// ledger keeps the capability's own error, unextended.
func (t *UseCapabilityTool) recordCallFailure(resolved tool.ResolvedCall, err error) error {
	if t.ledger != nil {
		t.ledger.MarkFailed(resolved.CapabilityID, err.Error())
	}
	if t.audit != nil {
		t.audit.RecordMCPProxy(false, true, true)
	}
	return fmt.Errorf("%w%s", err, contractHint(resolved.Target.Schema(), resolved.Args))
}
