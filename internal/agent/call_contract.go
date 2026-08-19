package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// argumentContract is how a call compares against its target's own schema.
type argumentContract struct {
	Accepts []string // every property the schema defines
	Unknown []string // supplied fields the schema does not define
	Missing []string // required fields the call omits
}

// readArgumentContract compares arguments against schema. ok is false when
// either side is unreadable, which leaves the contract unknown rather than
// violated — a caller may not treat that as a broken call.
func readArgumentContract(schema, arguments json.RawMessage) (argumentContract, bool) {
	var declared struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if len(schema) == 0 || json.Unmarshal(schema, &declared) != nil || len(declared.Properties) == 0 {
		return argumentContract{}, false
	}
	var got map[string]json.RawMessage
	if len(arguments) > 0 && json.Unmarshal(arguments, &got) != nil {
		return argumentContract{}, false
	}
	contract := argumentContract{Accepts: slices.Sorted(maps.Keys(declared.Properties))}
	for field := range got {
		if _, defined := declared.Properties[field]; !defined {
			contract.Unknown = append(contract.Unknown, field)
		}
	}
	for _, field := range declared.Required {
		if _, supplied := got[field]; !supplied {
			contract.Missing = append(contract.Missing, field)
		}
	}
	slices.Sort(contract.Unknown)
	slices.Sort(contract.Missing)
	return contract, true
}

// hint names the parameters the target accepts, but only when the call actually
// broke the contract. Staying silent on a well-formed call keeps the contract
// out of failures it cannot explain, such as one the target itself reported.
func (c argumentContract) hint() string {
	if len(c.Unknown) == 0 && len(c.Missing) == 0 {
		return ""
	}
	var b strings.Builder
	if len(c.Unknown) > 0 {
		fmt.Fprintf(&b, "; %s is not a parameter of this capability", strings.Join(quoted(c.Unknown), ", "))
	}
	if len(c.Missing) > 0 {
		fmt.Fprintf(&b, "; it requires %s", strings.Join(quoted(c.Missing), ", "))
	}
	fmt.Fprintf(&b, "; it accepts %s", strings.Join(quoted(c.Accepts), ", "))
	return b.String()
}

// applyCallContract refuses a call that omits a field its target's schema
// requires. Execution was never possible, and running before permission keeps a
// user approval from being spent on a call that can only fail after it.
func (a *Agent) applyCallContract(plan *toolCallPlan) (toolOutcome, bool) {
	if plan.execTool == nil {
		return toolOutcome{}, false
	}
	contract, ok := readArgumentContract(plan.execTool.Schema(), plan.execArgs)
	if !ok || len(contract.Missing) == 0 {
		return toolOutcome{}, false
	}
	msg := fmt.Sprintf("invalid arguments for %s%s", plan.permName, contract.hint())
	return toolOutcome{output: "error: " + msg, errMsg: msg}, true
}
