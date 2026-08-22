package builtin

import (
	"encoding/json"
	"sort"
	"testing"

	"reasonix/internal/tokencount"
	"reasonix/internal/tool"
)

// The largest single item in a first request — 3,995 of one measured run's
// 5,577 prompt tokens — and nothing was watching it: a field here, a sentence
// there, and the floor under every session rises unnoticed. The whole built-in
// set rather than a preset's slice of it, so the number does not move when an
// allow-list is edited.
const builtinSchemaTokenBudget = 5700

func TestBuiltinSchemaSurfaceStaysWithinBudget(t *testing.T) {
	r := tool.NewRegistry()
	for _, x := range tool.Builtins() {
		r.Add(x)
	}
	type row struct {
		name string
		tok  int
	}
	var rows []row
	total := 0
	for _, s := range r.Schemas() {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		n := tokencount.Text(string(b))
		rows = append(rows, row{s.Name, n})
		total += n
	}
	// A registry that came up empty would pass any ceiling.
	if len(rows) < 15 {
		t.Fatalf("only %d built-in schemas were read; the registry is not being populated here", len(rows))
	}
	if total > builtinSchemaTokenBudget {
		sort.Slice(rows, func(i, j int) bool { return rows[i].tok > rows[j].tok })
		t.Errorf("built-in tool schemas are %d tokens, over the %d budget by %d.\n"+
			"Raise the budget only with a reason: every session pays this before it does anything.\nLargest:", total, builtinSchemaTokenBudget, total-builtinSchemaTokenBudget)
		for _, x := range rows[:8] {
			t.Errorf("  %5d  %s", x.tok, x.name)
		}
	}
}
