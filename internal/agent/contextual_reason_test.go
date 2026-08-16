package agent

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/tool"
)

// The same contract internal/tool enforces over the built-ins, over the tools
// this package owns: a tool that can be out of context says what would put it
// back in. Listed explicitly because these are constructed by the host rather
// than registered globally — a new one here has to be added to this list, which
// is the point at which its author is asked for the reason.
func TestAgentContextualToolsExplainThemselves(t *testing.T) {
	for _, target := range []tool.Tool{&SubmitPlanTool{}, &ConcludeNoChangesTool{}} {
		if _, ok := target.(tool.ContextualTool); !ok {
			t.Errorf("%s is listed here but is not contextual", target.Name())
			continue
		}
		r, ok := target.(tool.ContextualReasoner)
		if !ok {
			t.Errorf("%s cannot say why it is unavailable", target.Name())
			continue
		}
		if strings.TrimSpace(r.Unavailable(context.Background())) == "" {
			t.Errorf("%s returned a blank reason", target.Name())
		}
	}
}
