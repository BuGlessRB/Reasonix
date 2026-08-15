package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/tool"
)

const maxRecallPositions = 8

func init() { tool.RegisterBuiltin(recallContext{}) }

type recallContext struct{}

func (recallContext) Name() string { return "recall" }

func (recallContext) Description() string {
	return "Bring a folded part of this conversation back, addressed by the #n positions listed under \"Folded work index\" in a compaction summary. Re-run a read or a search instead when the file or the code is what you want — a current copy beats a folded one. Recall is for what you cannot re-derive: what the user said in their own words, and the output of work you should not repeat. Each compaction grants a fresh budget; a request past it is refused whole, not truncated."
}

func (recallContext) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"additionalProperties":false,
"properties":{
  "positions":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"integer","minimum":0},"description":"The #n addresses from a folded-work index line, without the '#'."}
},
"required":["positions"]
}`)
}

// ReadOnly is true in the permission/workspace sense: recall only reads this
// session's own canonical transcript.
func (recallContext) ReadOnly() bool { return true }

func (recallContext) PlanModeSafe() bool { return true }

func (recallContext) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var request struct {
		Positions []int `json:"positions"`
	}
	if err := json.Unmarshal(args, &request); err != nil {
		return "", fmt.Errorf("invalid recall args: %w", err)
	}
	if len(request.Positions) == 0 {
		return "", fmt.Errorf("recall: positions must name at least one #n address")
	}
	if len(request.Positions) > maxRecallPositions {
		return "", fmt.Errorf("recall: at most %d positions per call", maxRecallPositions)
	}
	recaller, ok := tool.ContextRecallerFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("recall is unavailable outside an active agent session")
	}
	result, err := recaller.RecallContext(ctx, tool.RecallRequest{Positions: request.Positions})
	if err != nil {
		return "", err
	}
	return renderRecall(result), nil
}

// renderRecall puts the accounting last: the content is what the model is here
// to read, and a footer keeps the budget visible without framing every line.
func renderRecall(res tool.RecallResult) string {
	var b strings.Builder
	b.WriteString(res.Text)
	if !strings.HasSuffix(res.Text, "\n") {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n[recalled %s; %d tokens; %d left before the next compaction",
		positionList(res.Recalled), res.Tokens, res.BudgetLeft)
	if len(res.Missing) > 0 {
		fmt.Fprintf(&b, "; no longer addressable: %s", positionList(res.Missing))
	}
	b.WriteString("]")
	return b.String()
}

func positionList(positions []int) string {
	if len(positions) == 0 {
		return "nothing"
	}
	parts := make([]string, len(positions))
	for i, p := range positions {
		parts[i] = fmt.Sprintf("#%d", p)
	}
	return strings.Join(parts, ", ")
}
