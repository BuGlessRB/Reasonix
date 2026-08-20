package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// argumentContract is how a call compares against its target's own schema.
type argumentContract struct {
	Level   string   // the array property whose items were compared; empty is the call itself
	Accepts []string // every property the schema defines
	Unknown []string // supplied fields the schema does not define
	Missing []string // required fields the call omits
}

// schemaLevel is one object level of a JSON Schema: what it defines and what
// it insists on.
type schemaLevel struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}

// readArgumentContract compares arguments against schema, descending into array
// items once the call's own level is clean: a field named inside `tasks[]` has
// to be answered with an item's parameters, not with the outer object's. ok is
// false when either side is unreadable, which leaves the contract unknown rather
// than violated — a caller may not treat that as a broken call.
func readArgumentContract(schema, arguments json.RawMessage) (argumentContract, bool) {
	var declared schemaLevel
	if len(schema) == 0 || json.Unmarshal(schema, &declared) != nil || len(declared.Properties) == 0 {
		return argumentContract{}, false
	}
	var got map[string]json.RawMessage
	if len(arguments) > 0 && json.Unmarshal(arguments, &got) != nil {
		return argumentContract{}, false
	}
	contract := declared.compare(got, "")
	if contract.broken() {
		return contract, true
	}
	for _, field := range slices.Sorted(maps.Keys(declared.Properties)) {
		if nested, found := itemContract(declared.Properties[field], got[field], field); found {
			return nested, true
		}
	}
	return contract, true
}

func (s schemaLevel) compare(got map[string]json.RawMessage, level string) argumentContract {
	contract := argumentContract{Level: level, Accepts: slices.Sorted(maps.Keys(s.Properties))}
	for field := range got {
		if _, defined := s.Properties[field]; !defined {
			contract.Unknown = append(contract.Unknown, field)
		}
	}
	for _, field := range s.Required {
		if _, supplied := got[field]; !supplied {
			contract.Missing = append(contract.Missing, field)
		}
	}
	slices.Sort(contract.Unknown)
	slices.Sort(contract.Missing)
	return contract
}

func (c argumentContract) broken() bool { return len(c.Unknown) > 0 || len(c.Missing) > 0 }

// itemContract compares the supplied elements of an array property against that
// property's item schema and returns the first element that breaks it.
func itemContract(property, value json.RawMessage, field string) (argumentContract, bool) {
	var wrapper struct {
		Items json.RawMessage `json:"items"`
	}
	if len(property) == 0 || json.Unmarshal(property, &wrapper) != nil || len(wrapper.Items) == 0 {
		return argumentContract{}, false
	}
	var items schemaLevel
	if json.Unmarshal(wrapper.Items, &items) != nil || len(items.Properties) == 0 {
		return argumentContract{}, false
	}
	var elements []map[string]json.RawMessage
	if len(value) == 0 || json.Unmarshal(value, &elements) != nil {
		return argumentContract{}, false
	}
	for _, element := range elements {
		if c := items.compare(element, field); c.broken() {
			return c, true
		}
	}
	return argumentContract{}, false
}

// hint names the parameters the target accepts, but only when the call actually
// broke the contract. Staying silent on a well-formed call keeps the contract
// out of failures it cannot explain, such as one the target itself reported.
func (c argumentContract) hint() string {
	if len(c.Unknown) == 0 && len(c.Missing) == 0 {
		return ""
	}
	subject, owner := "this capability", "it"
	if c.Level != "" {
		subject = "a `" + c.Level + "` item"
		owner = subject
	}
	var b strings.Builder
	if len(c.Unknown) > 0 {
		fmt.Fprintf(&b, "; %s is not a parameter of %s", strings.Join(quoted(c.Unknown), ", "), subject)
	}
	if len(c.Missing) > 0 {
		fmt.Fprintf(&b, "; %s requires %s", owner, strings.Join(quoted(c.Missing), ", "))
	}
	fmt.Fprintf(&b, "; %s accepts %s", owner, strings.Join(quoted(c.Accepts), ", "))
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

// malformedArgumentsDetail says why the arguments did not parse. A call that
// was cut off mid-value and one that mistyped an escape need opposite fixes —
// send less, versus escape correctly — and a bare "not valid JSON" sends both
// to the wrong one.
func malformedArgumentsDetail(arguments string) string {
	const reemit = " Re-emit them exactly per this schema:"
	var probe any
	err := json.NewDecoder(strings.NewReader(arguments)).Decode(&probe)
	switch syntax := (*json.SyntaxError)(nil); {
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "The arguments end mid-value: they were cut off, not mistyped, so re-sending the same shape truncates again. Keep summaries and free text short." + reemit
	case errors.As(err, &syntax):
		return fmt.Sprintf("The arguments were not valid JSON at byte %d: %v.%s", syntax.Offset, err, reemit)
	}
	return "The arguments were not valid JSON." + reemit
}
