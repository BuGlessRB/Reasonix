package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// argumentContract is how a call compares against its target's own schema.
type argumentContract struct {
	Level   string   // the array property whose items were compared; empty is the call itself
	Accepts []string // every property the schema defines
	Unknown []string // supplied fields the schema does not define
	Missing []string // required fields the call omits
	// Mistyped are supplied fields the schema defines and the call filled with
	// the wrong kind of value. Neither unknown nor missing: the name is right
	// and it is there, so only the value can be.
	Mistyped []mistypedField
}

// mistypedField is one field whose value is not a kind its schema admits.
type mistypedField struct {
	Field string
	Want  []string
	Got   string
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
	for _, field := range slices.Sorted(maps.Keys(got)) {
		property, defined := s.Properties[field]
		if !defined {
			contract.Unknown = append(contract.Unknown, field)
			continue
		}
		if want, actual, bad := kindMismatch(property, got[field]); bad {
			contract.Mistyped = append(contract.Mistyped, mistypedField{Field: field, Want: want, Got: actual})
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

func (c argumentContract) broken() bool {
	return len(c.Unknown) > 0 || len(c.Missing) > 0 || len(c.Mistyped) > 0
}

// kindMismatch reports whether value is a kind this property does not admit. A
// property that declares no type, and a null, leave the question unanswered:
// the contract may only report what the schema actually said.
func kindMismatch(property, value json.RawMessage) (want []string, got string, bad bool) {
	want = declaredTypes(property)
	got = jsonKind(value)
	if len(want) == 0 || got == "" || got == "null" {
		return nil, "", false
	}
	for _, kind := range want {
		if kind == got || (got == "number" && (kind == "number" || kind == "integer")) {
			return nil, "", false
		}
	}
	return want, got, true
}

// declaredTypes reads a property's "type", which JSON Schema allows to be one
// name or several.
func declaredTypes(property json.RawMessage) []string {
	var one struct {
		Type json.RawMessage `json:"type"`
	}
	if len(property) == 0 || json.Unmarshal(property, &one) != nil || len(one.Type) == 0 {
		return nil
	}
	var single string
	if json.Unmarshal(one.Type, &single) == nil {
		return []string{single}
	}
	var several []string
	if json.Unmarshal(one.Type, &several) == nil {
		return several
	}
	return nil
}

// jsonKind names the kind of an encoded value by its first token, which is what
// a JSON Schema type names too.
func jsonKind(value json.RawMessage) string {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		return ""
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

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
	if !c.broken() {
		return ""
	}
	subject, owner := "this capability", "it"
	where := ""
	if c.Level != "" {
		subject = "a `" + c.Level + "` item"
		owner, where = subject, " in "+subject
	}
	var b strings.Builder
	if len(c.Unknown) > 0 {
		fmt.Fprintf(&b, "; %s is not a parameter of %s", strings.Join(quoted(c.Unknown), ", "), subject)
	}
	if len(c.Missing) > 0 {
		fmt.Fprintf(&b, "; %s requires %s", owner, strings.Join(quoted(c.Missing), ", "))
	}
	for _, m := range c.Mistyped {
		fmt.Fprintf(&b, "; %q%s must be %s, not %s", m.Field, where, kindPhrase(m.Want), kindPhrase([]string{m.Got}))
	}
	// A mistyped field is one the target accepts, so listing what it accepts
	// answers a question nobody asked.
	if len(c.Unknown) > 0 || len(c.Missing) > 0 {
		fmt.Fprintf(&b, "; %s accepts %s", owner, strings.Join(quoted(c.Accepts), ", "))
	}
	return b.String()
}

// kindPhrase reads a JSON Schema type list the way a sentence needs it.
func kindPhrase(kinds []string) string {
	articled := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		switch kind {
		case "array", "object", "integer":
			articled = append(articled, "an "+kind)
		default:
			articled = append(articled, "a "+kind)
		}
	}
	return strings.Join(articled, " or ")
}

// applyCallContract refuses a call the target's own schema already rules out: a
// required field omitted, or one filled with a kind the schema does not admit.
// Execution was never possible, so this keeps a user approval from being spent
// on a call that can only fail after it, and keeps the answer from being an
// unmarshal error written in the host language's vocabulary.
func (a *Agent) applyCallContract(plan *toolCallPlan) (toolOutcome, bool) {
	if plan.execTool == nil {
		return toolOutcome{}, false
	}
	contract, ok := readArgumentContract(plan.execTool.Schema(), plan.execArgs)
	if !ok || (len(contract.Missing) == 0 && len(contract.Mistyped) == 0) {
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
		return fmt.Sprintf("The arguments were not valid JSON at byte %d: %v.%s%s",
			syntax.Offset, err, argumentsExcerpt(arguments, syntax.Offset), reemit)
	}
	return "The arguments were not valid JSON." + reemit
}

// argumentsExcerpt shows the bytes the parser stopped on. An offset into a
// payload the model cannot see leaves it explaining the failure with whatever
// it *can* see — which is how a missing delimiter is learned as "the host
// rejects angle brackets". The closing line says what the caret does and does
// not prove, so the excerpt cannot teach a rule of its own.
func argumentsExcerpt(arguments string, offset int64) string {
	at := int(offset) - 1 // Offset counts bytes consumed, so the byte read last precedes it
	at = min(max(at, 0), len(arguments)-1)
	if at < 0 {
		return ""
	}
	const flank = 48
	lo, hi := max(at-flank, 0), min(at+flank+1, len(arguments))
	for lo > 0 && !utf8.RuneStart(arguments[lo]) {
		lo--
	}
	for hi < len(arguments) && !utf8.RuneStart(arguments[hi]) {
		hi++
	}

	var line strings.Builder
	if lo > 0 {
		line.WriteString("…")
	}
	caret := -1
	for i := lo; i < hi; {
		r, size := utf8.DecodeRuneInString(arguments[i:])
		if caret < 0 && i <= at && at < i+size {
			caret = visibleWidth(line.String())
		}
		line.WriteString(escapeExcerptRune(r))
		i += size
	}
	if hi < len(arguments) {
		line.WriteString("…")
	}
	if caret < 0 {
		caret = visibleWidth(line.String())
	}
	return fmt.Sprintf("\n\n  %s\n  %s^\n\nThe caret marks where parsing stopped, not necessarily the character to remove: any byte is legal inside a JSON string, so when one shows up in the syntax around the strings it is the quoting or a delimiter before it that is missing.",
		line.String(), strings.Repeat(" ", caret))
}

// escapeExcerptRune keeps an excerpt to one line so the caret below it stays
// aligned with the byte it points at.
func escapeExcerptRune(r rune) string {
	switch r {
	case '\n':
		return "\\n"
	case '\r':
		return "\\r"
	case '\t':
		return "\\t"
	}
	if r < 0x20 || r == 0x7f {
		return fmt.Sprintf("\\x%02x", r)
	}
	return string(r)
}
