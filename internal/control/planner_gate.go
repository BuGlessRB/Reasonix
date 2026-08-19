package control

import (
	"context"
	"strings"
	"unicode"

	"reasonix/internal/agent"
	"reasonix/internal/agentpreset"
	"reasonix/internal/taskpolicy"
)

// The planner gate answers one question: did the user ask for a plan? Twelve
// keyword tables used to score the answer out of vocabulary; the executor reads
// the same words with more context and plans for itself when a task needs it.

const (
	plannerLightResearchRounds = 2
	plannerFullResearchRounds  = 6
)

const (
	plannerReasonExplicitPlanMode   = "explicit_plan_mode"
	plannerReasonSynthetic          = "synthetic"
	plannerReasonSlash              = "slash_command"
	plannerReasonEmpty              = "empty_turn"
	plannerReasonUserPlanOnly       = "user_plan_only"
	plannerReasonUserPlanAndExecute = "user_plan_and_execute"
	plannerReasonExecutorOwns       = "executor_owns_the_turn"
)

type plannerTurnMetadata struct {
	UserText         string
	Synthetic        bool
	ExplicitPlanMode bool
	Policy           taskpolicy.TaskPolicy
	PolicySet        bool
}

type plannerTurnMetadataKey struct{}

func withPlannerTurnMetadata(ctx context.Context, meta plannerTurnMetadata) context.Context {
	return context.WithValue(ctx, plannerTurnMetadataKey{}, meta)
}

func plannerTurnMetadataFromContext(ctx context.Context) (plannerTurnMetadata, bool) {
	if ctx == nil {
		return plannerTurnMetadata{}, false
	}
	meta, ok := ctx.Value(plannerTurnMetadataKey{}).(plannerTurnMetadata)
	return meta, ok
}

func (c *Controller) withPlannerTurnMetadata(ctx context.Context, userText string, synthetic bool) context.Context {
	policy := taskpolicy.Derive(taskpolicy.Input{
		Preset:   agentpreset.Normalize(c.AgentPreset()),
		PlanMode: c.PlanMode(),
	})
	ctx = taskpolicy.WithContext(ctx, policy)
	return withPlannerTurnMetadata(ctx, plannerTurnMetadata{
		UserText:         userText,
		Synthetic:        synthetic,
		ExplicitPlanMode: c.PlanMode(),
		Policy:           policy,
		PolicySet:        true,
	})
}

// DecidePlannerRoute routes a turn to the two-model planner only when the user
// asked for a plan. It never calls a model and never scores the task text.
func DecidePlannerRoute(ctx context.Context, input string) agent.PlannerDecision {
	meta, hasMeta := plannerTurnMetadataFromContext(ctx)
	composedText := strings.TrimSpace(agent.StripTransientUserBlocks(input))
	text := composedText
	if hasMeta && strings.TrimSpace(meta.UserText) != "" {
		text = strings.TrimSpace(meta.UserText)
	}

	if meta.ExplicitPlanMode || strings.HasPrefix(composedText, PlanModeMarker) {
		return plannerExecutorDecision(plannerReasonExplicitPlanMode)
	}
	if meta.Synthetic || IsSyntheticUserMessage(text) {
		return plannerExecutorDecision(plannerReasonSynthetic)
	}
	if text == "" {
		return plannerExecutorDecision(plannerReasonEmpty)
	}
	if strings.HasPrefix(text, "/") {
		return plannerExecutorDecision(plannerReasonSlash)
	}

	lower := normalizePlannerText(text)
	if requestsPlanOnly(lower) {
		return plannerPlanDecision(agent.PlannerRoutePlanOnly, agent.PlannerDepthFull, plannerReasonUserPlanOnly)
	}
	if hasLeadingDirective(lower, planAndExecuteDirectives) || hasLeadingDirective(lower, planFirstDirectives) {
		return plannerPlanDecision(agent.PlannerRoutePlanAndExecute, agent.PlannerDepthFull, plannerReasonUserPlanAndExecute)
	}
	// Nobody asked for a plan. The executor decides for itself whether this
	// turn needs steps, and states them with todo_write where the delivery
	// gates and the contract can both read them.
	return plannerExecutorDecision(plannerReasonExecutorOwns)
}

func plannerExecutorDecision(reason string) agent.PlannerDecision {
	return agent.PlannerDecision{
		Route:  agent.PlannerRouteExecutorOnly,
		Depth:  agent.PlannerDepthNone,
		Reason: reason,
	}
}

func plannerPlanDecision(route agent.PlannerRoute, depth agent.PlannerDepth, reason string) agent.PlannerDecision {
	rounds := plannerLightResearchRounds
	if depth == agent.PlannerDepthFull {
		rounds = plannerFullResearchRounds
	}
	return agent.PlannerDecision{
		Route:             route,
		Depth:             depth,
		Reason:            reason,
		MaxResearchRounds: rounds,
	}
}

func normalizePlannerText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.ReplaceAll(text, "’", "'")
	return text
}

func hasLeadingDirective(lower string, directives []string) bool {
	lower = strings.TrimSpace(lower)
	for _, polite := range []string{"please ", "please, ", "请", "请先", "麻烦", "麻烦先"} {
		if after, ok := strings.CutPrefix(lower, polite); ok {
			lower = strings.TrimSpace(after)
			break
		}
	}
	for _, directive := range directives {
		if strings.HasPrefix(lower, directive) {
			return true
		}
	}
	return false
}

// The tables below read directives, not topics: each entry is a way of saying
// "plan this first" or "wait for my approval". They bind the host because the
// user said them, which is the difference from the scoring this file dropped.

var planAndExecuteDirectives = []string{
	"先规划再执行", "先规划再实现", "先出方案再执行", "先出方案再实现",
	"plan first, then", "plan first then", "plan then implement", "plan and implement",
}

var planFirstDirectives = []string{
	"先规划", "先给方案", "先出方案", "给我方案",
	"plan first", "draft a plan", "give me a plan", "make a plan",
}

var planOnlyDirectives = []string{
	"只规划", "只做规划", "只给方案", "只出方案", "给我方案即可",
	"plan only", "only plan", "just plan", "give me only a plan", "give me a plan only",
}

// requestsPlanOnly reads a leading directive, never the body. Free-text
// matching used to decide this and stopped the turn dead on requests that only
// mentioned planning: "add a test for the plan contract" and "document why we
// do not implement the plan cache" both routed to plan-only and did no work.
func requestsPlanOnly(lower string) bool {
	return hasLeadingDirective(plannerDirectiveText(lower), planOnlyDirectives)
}

// plannerDirectiveText removes quoted examples before applying execution
// boundaries. A user explaining "do not execute" or “别规划” is not issuing
// that directive. ASCII apostrophes inside words remain literal, so
// contractions such as don't keep matching the directive tables.
func plannerDirectiveText(text string) string {
	var b strings.Builder
	var closing rune
	escaped := false
	runes := []rune(text)
	for i, r := range runes {
		if closing != 0 {
			if escaped {
				escaped = false
				b.WriteRune(' ')
				continue
			}
			if (closing == '"' || closing == '`') && r == '\\' {
				escaped = true
				b.WriteRune(' ')
				continue
			}
			if r == closing && (closing != '\'' || !plannerInlineApostrophe(runes, i)) {
				closing = 0
			}
			b.WriteRune(' ')
			continue
		}
		switch r {
		case '"':
			closing = '"'
			b.WriteRune(' ')
		case '“':
			closing = '”'
			b.WriteRune(' ')
		case '‘':
			// normalizePlannerText converts the closing ’ to ASCII '.
			closing = '\''
			b.WriteRune(' ')
		case '\'':
			if plannerSingleQuoteStart(runes, i) {
				closing = '\''
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(r)
		case '`':
			closing = '`'
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func plannerSingleQuoteStart(runes []rune, i int) bool {
	if i+1 >= len(runes) || !unicode.IsLetter(runes[i+1]) {
		return false
	}
	return i == 0 || !unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1])
}

func plannerInlineApostrophe(runes []rune, i int) bool {
	return i > 0 && i+1 < len(runes) &&
		(unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1])) &&
		(unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1]))
}

// TaskWarrantsPlanner is retained as a small compatibility predicate for
// callers and tests that do not need depth or approval semantics.
func TaskWarrantsPlanner(input string) bool {
	return DecidePlannerRoute(context.Background(), input).Route != agent.PlannerRouteExecutorOnly
}

// NewPlannerPolicy returns the structured deterministic policy used by the
// two-model product path.
func NewPlannerPolicy() agent.PlannerPolicy {
	return DecidePlannerRoute
}

// NewPlannerGate retains the historical bool shape for direct callers.
func NewPlannerGate() func(context.Context, string) bool {
	return func(ctx context.Context, input string) bool {
		return DecidePlannerRoute(ctx, input).Route != agent.PlannerRouteExecutorOnly
	}
}
