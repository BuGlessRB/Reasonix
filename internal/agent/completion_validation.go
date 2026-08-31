package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/completioneval"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// Completion-validation modes. The values are fixed config strings; an invalid
// one is rejected at config load, and New maps anything else reaching this
// layer to off.
const (
	CompletionValidationOff     = "off"
	CompletionValidationShadow  = "shadow"
	CompletionValidationEnforce = "enforce"
)

func normalizeCompletionValidation(mode string) string {
	switch strings.TrimSpace(mode) {
	case CompletionValidationShadow, CompletionValidationEnforce:
		return strings.TrimSpace(mode)
	default:
		return CompletionValidationOff
	}
}

func resolveCompletionEvaluator(opts Options) completioneval.Evaluator {
	if opts.CompletionEvaluator != nil {
		return opts.CompletionEvaluator
	}
	if opts.CompletionEvaluatorFactory != nil {
		return opts.CompletionEvaluatorFactory()
	}
	return nil
}

// completionDecision is what the validator says the run should do next.
type completionDecision int

const (
	// completionAccept ends the run normally: the validator confirmed the
	// candidate, or the mode decided not to apply.
	completionAccept completionDecision = iota
	// completionResume continues the same Run after one continuation tail.
	completionResume
	// completionStop ends the run in a recoverable completion pause.
	completionStop
)

// completionEnforced reports whether this agent strictly applies the validator.
func (a *Agent) completionEnforced() bool {
	return a.completionValidation == CompletionValidationEnforce && a.completionEvaluator != nil
}

// validatorApplies reports whether this candidate terminal should be validated.
// Active Goal turns are excluded: the Goal FSM already owns their continuation
// through its own evaluator, and a second opinion here would double the calls.
func (a *Agent) validatorApplies(ctx context.Context) bool {
	if a.completionValidation == CompletionValidationOff || a.completionEvaluator == nil {
		return false
	}
	if _, goalScoped := tool.GoalTurnRecorderFromContext(ctx); goalScoped {
		return false
	}
	return true
}

// validateCandidateCompletion runs the completion validator on a candidate
// terminal turn and advances the terminal-protocol phase. It bounds the
// protocol: at most one continuation per Run (initial -> repairing), then the
// run either validates or pauses. shadow mode records the verdict but never
// changes the outcome.
func (a *Agent) validateCandidateCompletion(ctx context.Context, state *turnRuntime, candidate string) (completionDecision, *CompletionUncertainError) {
	if !a.validatorApplies(ctx) {
		return completionAccept, nil
	}
	switch state.terminal.validation {
	case completionValidated, completionPaused:
		return completionAccept, nil
	}
	attempt := 1
	if state.terminal.validation == completionRepairing {
		attempt = 2
	}
	mode := a.completionValidation
	started := time.Now()
	verdict, err := a.completionEvaluator.Evaluate(ctx, a.completionEvidence(ctx, candidate))
	duration := time.Since(started)

	outcome, errClass := validatorOutcome(verdict, err)
	a.svc.sink.Emit(event.Event{
		Kind: event.CompletionValidation,
		CompletionValidation: &event.CompletionValidationInfo{
			Mode: mode, Outcome: outcome, Attempt: attempt,
			DurationMs: duration.Milliseconds(), ErrorClass: errClass,
		},
	})
	if mode == CompletionValidationShadow {
		return completionAccept, nil
	}
	if err != nil {
		state.terminal.validation = completionPaused
		return completionStop, &CompletionUncertainError{
			Cause:  CompletionUncertainValidatorFailed,
			Detail: errClass,
		}
	}
	switch verdict.Outcome {
	case completioneval.OutcomeComplete, completioneval.OutcomeNeedsUser, completioneval.OutcomeBlocked:
		state.terminal.validation = completionValidated
		return completionAccept, nil
	case completioneval.OutcomeContinue:
		if state.terminal.validation == completionRepairing {
			state.terminal.validation = completionPaused
			return completionStop, &CompletionUncertainError{Cause: CompletionUncertainValidatorContinue}
		}
		state.terminal.validation = completionRepairing
		a.sess.conversation.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: a.withTurnPreferences(completionContinueTailMessage()),
		})
		return completionResume, nil
	default: // uncertain
		state.terminal.validation = completionPaused
		return completionStop, &CompletionUncertainError{Cause: CompletionUncertainValidatorUncertain}
	}
}

func validatorOutcome(verdict completioneval.Verdict, err error) (outcome, errClass string) {
	if err != nil {
		return "error", validatorErrorClass(err)
	}
	return string(verdict.Outcome), ""
}

func validatorErrorClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, completioneval.ErrInvalidOutput):
		return "invalid_output"
	case strings.Contains(err.Error(), "unavailable"):
		return "unavailable"
	case strings.Contains(err.Error(), "exceeds"):
		return "over_budget"
	default:
		return "error"
	}
}

func completionContinueTailMessage() string {
	return "The host could not confirm this turn is complete: the last message did not deliver a self-contained final result. If the request is already satisfied, reply now with the complete final answer. Otherwise continue the work and finish with the complete final answer; do not stop with only a summary of intentions."
}

// completionEvidence assembles the validator's structured evidence from
// host-owned state. Nothing here reads the model's wording for keywords; every
// field is either host-authored or passed through as untrusted data.
func (a *Agent) completionEvidence(ctx context.Context, candidate string) completioneval.Evidence {
	return completioneval.Evidence{
		TaskText:        a.turn.turnInput,
		RecentTurns:     a.recentVisibleTurns(),
		CandidateAnswer: candidate,
		Mode:            a.completionModeLabel(ctx),
		HostSummary:     a.completionHostSummary(),
	}
}

// completionModeLabel names the workflow phase for the evidence payload. It is
// a host-authored label, never read back for control flow.
func (a *Agent) completionModeLabel(ctx context.Context) string {
	switch {
	case a.subagentDepth > 0:
		return completioneval.ModeSubagent
	case a.turn.deliveryScopeActive:
		return completioneval.ModeDelivery
	case a.planMode.Load():
		return completioneval.ModePlan
	case a.usageSource == event.UsageSourcePlanner:
		return completioneval.ModePlanner
	default:
		return completioneval.ModeStandard
	}
}

// recentVisibleTurns collects the most recent visible user/assistant turns
// before the candidate answer, so references like "continue" or "that
// question" stay resolvable. The candidate itself is the conversation tail and
// is excluded — it travels in its own field.
func (a *Agent) recentVisibleTurns() []completioneval.ContextTurn {
	msgs := a.sess.conversation.Messages
	// The candidate assistant turn is the last message on a final-response path.
	start := len(msgs) - 1
	if start >= 0 && msgs[start].Role == provider.RoleAssistant {
		start--
	}
	turns := make([]completioneval.ContextTurn, 0, completioneval.MaxRecentTurns)
	for i := start; i >= 0 && len(turns) < completioneval.MaxRecentTurns; i-- {
		m := msgs[i]
		if m.Role != provider.RoleUser && m.Role != provider.RoleAssistant {
			continue
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		turns = append(turns, completioneval.ContextTurn{Role: string(m.Role), Content: m.Content})
	}
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns
}

// completionHostSummary is the host-authored digest of observable work state:
// todo progress, readiness gaps, tool receipt outcomes, and spend. Counts and
// category ids only — no paths, commands, or tool output.
func (a *Agent) completionHostSummary() string {
	var parts []string
	if completed, tracking := a.canonicalTodoProgress(); tracking {
		parts = append(parts, fmt.Sprintf("todos: %d completed, some still open", completed))
	}
	if check := a.finalReadinessCheckFor(); check.applies {
		if check.reason != "" {
			parts = append(parts, "readiness gaps: "+check.reason)
		} else {
			parts = append(parts, "readiness: satisfied")
		}
	}
	if a.task.ledger != nil {
		succeeded, failed := 0, 0
		for _, receipt := range a.task.ledger.Receipts() {
			if receipt.Success {
				succeeded++
			} else {
				failed++
			}
		}
		parts = append(parts, fmt.Sprintf("tool receipts: %d succeeded, %d failed", succeeded, failed))
	}
	budget := a.task.budget
	parts = append(parts, fmt.Sprintf("spend: %d tool rounds, %d prompt tokens, %d output tokens",
		budget.rounds, budget.promptTokens, budget.outputTokens))
	return strings.Join(parts, "; ")
}
