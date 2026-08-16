package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// Observed live: a caller put capability_id inside "arguments" and retried four
// times, rearranging the nesting each round, because "capability_id is required"
// reads as "you did not send it" when it was sent one level too deep.
func TestMisplacedArgumentHintNamesTheNesting(t *testing.T) {
	got := misplacedArgumentHint(json.RawMessage(`{"capability_id":"tool:read_subagent_result","subagent_ref":"sa_1"}`), "capability_id")
	if !strings.Contains(got, "nested inside") {
		t.Fatalf("hint = %q, want it to say the value was nested", got)
	}
	if !strings.Contains(got, "capability_id") {
		t.Fatalf("hint = %q, want the field named", got)
	}
}

// A caller that genuinely omitted the field gets the plain requirement, with no
// invented claim about where it is.
func TestMisplacedArgumentHintStaysSilentWhenAbsent(t *testing.T) {
	for _, args := range []string{`{"subagent_ref":"sa_1"}`, `{}`, ``, `not json`, `[1,2]`} {
		if got := misplacedArgumentHint(json.RawMessage(args), "capability_id"); got != "" {
			t.Errorf("misplacedArgumentHint(%q) = %q, want no hint", args, got)
		}
	}
}
