package tool_test

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/tool"
	_ "reasonix/internal/tool/builtin"
)

// A tool that can be out of context has to be able to say what would put it
// back in. Without this the host falls back to "unavailable in the current
// workflow context", which gives the model nothing to act on — it burns a round
// finding out by trial, as a real session did against three different gates.
func TestContextualToolsExplainThemselves(t *testing.T) {
	ctx := context.Background()
	var checked int
	for _, target := range tool.Builtins() {
		if _, ok := target.(tool.ContextualTool); !ok {
			continue
		}
		checked++
		r, ok := target.(tool.ContextualReasoner)
		if !ok {
			t.Errorf("%s is a ContextualTool but cannot say why it is unavailable", target.Name())
			continue
		}
		if strings.TrimSpace(r.Unavailable(ctx)) == "" {
			t.Errorf("%s returned a blank reason", target.Name())
		}
	}
	if checked == 0 {
		t.Fatal("no contextual tools found — the registry or this test stopped seeing them")
	}
}
