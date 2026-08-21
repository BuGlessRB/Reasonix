package agent

import (
	"context"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/instruction"
	"reasonix/internal/jobs"
	"reasonix/internal/memory"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

// toolCallContext assembles the context one tool call runs under: the host
// collaborators it may reach, the evidence scope it writes into, and the turn
// preferences it must honor. Every binding is optional, so a degraded
// assembly loses the capability rather than the call.
func (a *Agent) toolCallContext(ctx context.Context, plan *toolCallPlan) context.Context {
	cctx := tool.WithContextCompressor(withCallContext(ctx, plan.call.ID, a.svc.sink, a.svc.asker, a.planMode.Load()), a)
	cctx = tool.WithContextRecaller(cctx, a)
	cctx = tool.WithContextBudgetReporter(cctx, a)
	cctx = WithSubagentDepth(cctx, a.subagentDepth)
	if a.task.ledger != nil {
		cctx = evidence.WithLedger(cctx, a.task.ledger)
		cctx = evidence.WithSessionMessages(cctx, a.sess.conversation.Snapshot)
		if a.deliveryProfile {
			cctx = evidence.WithDeliveryProfile(cctx)
		}
	}
	if !a.planMode.Load() {
		cctx = a.withContractState(cctx)
	}
	if plan.planReplacementAuthorized {
		cctx = tool.WithPlanReplacementAuthorization(cctx)
	}
	if len(a.projectChecks) > 0 {
		cctx = instruction.WithChecks(cctx, a.projectChecks)
	}
	if a.svc.jobs != nil {
		cctx = jobs.WithManager(cctx, a.svc.jobs)
	}
	if a.svc.sandboxEscape != nil {
		cctx = sandbox.WithEscapeApprover(cctx, a.svc.sandboxEscape)
	}
	if a.svc.configWrite != nil {
		cctx = tool.WithConfigWriteApprover(cctx, a.svc.configWrite)
	}
	if v := a.responseLanguage.Load(); v != nil {
		if lang, ok := v.(string); ok {
			cctx = WithResponseLanguagePreference(cctx, lang)
		}
	}
	if v := a.reasoningLanguage.Load(); v != nil {
		if lang, ok := v.(string); ok {
			cctx = WithReasoningLanguagePreference(cctx, lang)
		}
	}
	if a.svc.memQueue != nil {
		cctx = memory.WithQueue(cctx, a.svc.memQueue)
	}
	callID := plan.call.ID
	cctx = tool.WithProgress(cctx, func(chunk string) {
		a.svc.sink.Emit(event.Event{Kind: event.ToolProgress, Tool: event.Tool{ID: callID, Output: chunk}})
	})
	return cctx
}
