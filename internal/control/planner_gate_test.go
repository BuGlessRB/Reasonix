package control

import (
	"context"
	"testing"

	"reasonix/internal/agent"
)

// The gate answers "did the user ask for a plan?", so a turn that reads as
// work — however large, however risky-sounding — still belongs to the executor
// until someone says otherwise. These are the inputs the retired scoring layer
// used to buy a planner round for.
func TestWorkShapedTurnsStayWithTheExecutor(t *testing.T) {
	for _, input := range []string{
		"fix the bug",
		"add a login button",
		"执行修复",
		"开始迁移",
		"继续重构",
		"implement the new caching layer across the backend",
		"migrate authentication to the new provider and update every caller",
		"refactor parser.go, reader.go and writer.go so they share one lexer",
		"drop the legacy table in production and backfill from the archive",
		"1. read the config\n2. rewrite the loader\n3. run the tests",
		"how do I implement a new caching layer",
		"what's the best way to refactor this module",
		"what does this function do?",
		"解释一下这段代码",
		"帮我看一下这个报错",
		"review this PR",
		"run the tests",
		"继续",
		"好的",
	} {
		if got := DecidePlannerRoute(context.Background(), input); got.Route != agent.PlannerRouteExecutorOnly {
			t.Errorf("%q routed to %s (%s); nobody asked for a plan", input, got.Route, got.Reason)
		}
	}
}

// Host facts still short-circuit: plan mode owns its own workflow, and a turn
// the host synthesized is not a user asking for anything.
func TestHostFactsRouteToTheExecutor(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		meta   plannerTurnMetadata
		reason string
	}{
		{"plan mode marker", PlanModeMarker + "\n\n先规划再执行：重写解析器", plannerTurnMetadata{}, plannerReasonExplicitPlanMode},
		{"plan mode flag", "先规划再执行：重写解析器", plannerTurnMetadata{ExplicitPlanMode: true}, plannerReasonExplicitPlanMode},
		{"synthetic turn", goalContinueTurn, plannerTurnMetadata{}, plannerReasonSynthetic},
		{"synthetic flag", "先规划再执行：重写解析器", plannerTurnMetadata{Synthetic: true}, plannerReasonSynthetic},
		{"empty", "   ", plannerTurnMetadata{}, plannerReasonEmpty},
		{"slash command", "/init", plannerTurnMetadata{}, plannerReasonSlash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecidePlannerRoute(withPlannerTurnMetadata(context.Background(), tc.meta), tc.input)
			if got.Route != agent.PlannerRouteExecutorOnly || got.Reason != tc.reason {
				t.Fatalf("decision = %+v, want executor-only for %s", got, tc.reason)
			}
		})
	}
}

// What still buys a planner round: the user saying so.
func TestStatedDirectivesRouteToThePlanner(t *testing.T) {
	cases := []struct {
		name  string
		input string
		route agent.PlannerRoute
	}{
		{"plan first then execute", "先规划再执行：重写解析器", agent.PlannerRoutePlanAndExecute},
		{"plan first en", "plan first, then rewrite the parser", agent.PlannerRoutePlanAndExecute},
		{"make a plan", "make a plan for the parser rewrite", agent.PlannerRoutePlanAndExecute},
		{"plan only", "只给方案，不要执行", agent.PlannerRoutePlanOnly},
		{"plan only en", "give me a plan only for the parser rewrite", agent.PlannerRoutePlanOnly},
		// Asking to approve first routes to the planner; whether the turn stops
		// for approval is the plan contract's RequiresApproval, set by the
		// planner after it read the request, not the host matching phrases.
		{"plan then approve", "给我方案，等我确认后再执行", agent.PlannerRoutePlanAndExecute},
		{"plan then approve en", "draft a plan and wait for my approval", agent.PlannerRoutePlanAndExecute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecidePlannerRoute(context.Background(), tc.input)
			if got.Route != tc.route {
				t.Fatalf("decision = %+v, want %s", got, tc.route)
			}
			if got.MaxResearchRounds <= 0 {
				t.Fatalf("planned decision has no research budget: %+v", got)
			}
		})
	}
}

// A directive quoted as an example is the user describing one, not issuing one.
func TestQuotedDirectivesDoNotBind(t *testing.T) {
	for _, input := range []string{
		`the docs say "plan first, then implement" — is that still true?`,
		`用户说“先规划再执行”是什么意思`,
	} {
		if got := DecidePlannerRoute(context.Background(), input); got.Route != agent.PlannerRouteExecutorOnly {
			t.Errorf("%q routed to %s from a quoted directive", input, got.Route)
		}
	}
}

// Routing reads the pristine user text, never the blocks the controller wrapped
// around it: a goal block naming a migration must not speak for the user.
func TestPlannerPolicyUsesPristineMetadataInsteadOfInjectedContext(t *testing.T) {
	ctx := withPlannerTurnMetadata(context.Background(), plannerTurnMetadata{
		UserText: "fix typo in README",
	})
	input := activeGoalBlock("先规划再执行：migrate authentication") +
		"\n\n<capability-route>\nhigh risk migration\n</capability-route>\n\nfix typo in README"
	got := DecidePlannerRoute(ctx, input)
	if got.Route != agent.PlannerRouteExecutorOnly || got.Reason != plannerReasonExecutorOwns {
		t.Fatalf("decision used injected context instead of pristine user text: %+v", got)
	}
}

func TestTaskWarrantsPlannerTracksTheRoute(t *testing.T) {
	if TaskWarrantsPlanner("fix the bug") {
		t.Error("an ordinary work request must not warrant the planner")
	}
	if !TaskWarrantsPlanner("先规划再执行：重写解析器") {
		t.Error("a stated plan-first directive must warrant the planner")
	}
}

func TestNewPlannerGateTracksTheRoute(t *testing.T) {
	gate := NewPlannerGate()
	if gate == nil {
		t.Fatal("NewPlannerGate returned nil")
	}
	if gate(context.Background(), "what is this?") {
		t.Error("planner gate should leave a question with the executor")
	}
	if !gate(context.Background(), "make a plan for the parser rewrite") {
		t.Error("planner gate should honour a stated plan request")
	}
}

// Free-text matching decided this before and stopped the turn dead on requests
// that only mentioned planning. plan-only ends the turn with no execution, so a
// false positive here is work silently not done.
func TestPlanningMentionedAsSubjectDoesNotStopTheTurn(t *testing.T) {
	for _, input := range []string{
		"Add a test for the plan contract that asserts we do not execute steps twice",
		"Document why we do not implement the plan cache",
		"实现一个方案对比页面，等我确认后再发布的那个流程有 bug",
		"The approval flow does not wait for my approval before executing the plan; fix it",
		"修一下计划任务不执行的问题",
	} {
		got := DecidePlannerRoute(context.Background(), input)
		if got.Route == agent.PlannerRoutePlanOnly || got.Route == agent.PlannerRoutePlanForApproval {
			t.Errorf("%q routed to %s: planning is the subject here, not the instruction", input, got.Route)
		}
	}
}
