package agent

import (
	"encoding/json"
	"fmt"
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
