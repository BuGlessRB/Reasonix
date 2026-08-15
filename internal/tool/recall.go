package tool

import "context"

// RecallRequest names folded transcript positions to bring back, by the #n
// addresses a compaction digest's folded-work index carries.
type RecallRequest struct {
	Positions []int
}

// RecallResult is one recall's content plus what it cost. Text is what the
// model reads; the counters let it see the generation's budget draining.
type RecallResult struct {
	Text       string
	Recalled   []int
	Missing    []int
	Tokens     int
	BudgetLeft int
}

// ContextRecaller is supplied by the Agent that owns the active session, the
// same way ContextCompressor is: the canonical transcript never leaves the
// agent, so a tool asks it for a position rather than reading a session file.
type ContextRecaller interface {
	RecallContext(context.Context, RecallRequest) (RecallResult, error)
}

type contextRecallerKey struct{}

// WithContextRecaller binds the active session's recaller to a tool call.
func WithContextRecaller(ctx context.Context, recaller ContextRecaller) context.Context {
	if recaller == nil {
		return ctx
	}
	return context.WithValue(ctx, contextRecallerKey{}, recaller)
}

// ContextRecallerFromContext returns the recaller bound to this tool call.
func ContextRecallerFromContext(ctx context.Context) (ContextRecaller, bool) {
	recaller, ok := ctx.Value(contextRecallerKey{}).(ContextRecaller)
	return recaller, ok && recaller != nil
}
