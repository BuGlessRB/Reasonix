package agentpreset

// PlannerRoute is how the host starts a turn.
type PlannerRoute uint8

const (
	// RouteDirect executes without an independent planner phase.
	RouteDirect PlannerRoute = iota
	// RouteLightPlan uses lightweight internal planning.
	RouteLightPlan
	// RouteFullPlan requires a complete plan before execution.
	RouteFullPlan
)

// VerificationLevel is how much post-mutation checking the host requires.
type VerificationLevel uint8

const (
	// VerifyNone requires no host verification commands.
	VerifyNone VerificationLevel = iota
	// VerifyTargeted runs the cheapest relevant checks and diff review.
	VerifyTargeted
	// VerifyFull requires every acceptance check for the current epoch.
	VerifyFull
)

// ReviewLevel is when an independent reviewer sub-agent must run.
type ReviewLevel uint8

const (
	// ReviewNone never starts an independent reviewer.
	ReviewNone ReviewLevel = iota
	// ReviewConditional starts a reviewer when evidence coverage is incomplete
	// or the change spans modules.
	ReviewConditional
	// ReviewForced always starts an independent reviewer.
	ReviewForced
	// ReviewForcedSecurity always starts reviewer plus security-review.
	ReviewForcedSecurity
)

// PlannerPolicy is the immutable planner strategy for a role setting.
type PlannerPolicy struct {
	// DirectOK allows zero-plan execution for simple low-risk work.
	DirectOK bool
	// PreferLightPlan uses light planning for multi-file or fuzzy scope.
	PreferLightPlan bool
	// FullPlanOnHighRisk forces a full plan for high-risk work.
	FullPlanOnHighRisk bool
	// FullPlanOnMediumRisk forces a full plan for medium-risk work.
	FullPlanOnMediumRisk bool
	// AllowExploreSubagent permits proactive explore/research sub-agents.
	AllowExploreSubagent bool
}

// CapabilityPolicy is how optional capabilities are routed.
type CapabilityPolicy struct {
	// SemanticRouterAllowed enables the LLM capability router when needed.
	SemanticRouterAllowed bool
}

// VerificationPolicy is post-mutation verification intensity.
type VerificationPolicy struct {
	Level VerificationLevel
}

// ReviewPolicy is independent reviewer intensity.
type ReviewPolicy struct {
	// LowRisk is the review level for low-risk mutations.
	LowRisk ReviewLevel
	// MediumRisk is the review level for medium-risk mutations.
	MediumRisk ReviewLevel
	// HighRisk is the review level for high-risk mutations. High-risk work always
	// elevates at least to ReviewForcedSecurity for safety classes.
	HighRisk ReviewLevel
}

// PresetPolicy is the compiled, immutable strategy for one AgentPreset.
type PresetPolicy struct {
	Preset             AgentPreset
	PlannerPolicy      PlannerPolicy
	CapabilityPolicy   CapabilityPolicy
	VerificationPolicy VerificationPolicy
	ReviewPolicy       ReviewPolicy
}

// PolicyOf returns the compiled strategy for preset. Unknown values use Balanced.
func PolicyOf(preset AgentPreset) PresetPolicy {
	switch Normalize(string(preset)) {
	case Delivery:
		return PresetPolicy{
			Preset: Delivery,
			PlannerPolicy: PlannerPolicy{
				DirectOK:             true, // low-risk atomic only
				PreferLightPlan:      false,
				FullPlanOnHighRisk:   true,
				FullPlanOnMediumRisk: true,
				AllowExploreSubagent: true,
			},
			CapabilityPolicy: CapabilityPolicy{
				SemanticRouterAllowed: true,
			},
			VerificationPolicy: VerificationPolicy{
				Level: VerifyFull,
			},
			ReviewPolicy: ReviewPolicy{
				LowRisk:    ReviewNone,
				MediumRisk: ReviewForced,
				HighRisk:   ReviewForcedSecurity,
			},
		}
	default:
		return PresetPolicy{
			Preset: Balanced,
			PlannerPolicy: PlannerPolicy{
				DirectOK:             true,
				PreferLightPlan:      true,
				FullPlanOnHighRisk:   true,
				FullPlanOnMediumRisk: false,
				AllowExploreSubagent: true,
			},
			CapabilityPolicy: CapabilityPolicy{
				SemanticRouterAllowed: true,
			},
			VerificationPolicy: VerificationPolicy{
				Level: VerifyTargeted,
			},
			ReviewPolicy: ReviewPolicy{
				LowRisk:    ReviewNone,
				MediumRisk: ReviewConditional,
				HighRisk:   ReviewForcedSecurity,
			},
		}
	}
}

// ReviewForRisk returns the review level required for risk under this policy.
// risk is 0=low, 1=medium, 2=high. Values outside that range clamp to high.
func (p PresetPolicy) ReviewForRisk(risk int, securityClass bool) ReviewLevel {
	var level ReviewLevel
	switch {
	case risk >= 2:
		level = p.ReviewPolicy.HighRisk
	case risk == 1:
		level = p.ReviewPolicy.MediumRisk
	default:
		level = p.ReviewPolicy.LowRisk
	}
	if securityClass && level < ReviewForcedSecurity {
		level = ReviewForcedSecurity
	}
	return level
}
