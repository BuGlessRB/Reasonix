package evidence

// A turn's investigation runway, in look-only rounds. Every round costs the
// same and what it produced pays part of that back: proving something buys
// rounds of slack, changing something pays for its own round, looking buys back
// half of one, and a round that produced nothing pays full price. No round
// counter and no cliff — a turn lasts exactly as long as it keeps producing.
const (
	runwayRoundCost  = 4
	yieldFalsifiable = 3 * runwayRoundCost // an observation that could have refuted the plan
	// A landed change is host-observable work and covers its round, so a turn
	// that keeps making them is never interrupted. That the change still owes
	// verification is real but already owned by the evidence-before-more-
	// mutation trigger; charging it here too would stop working turns.
	yieldChange = runwayRoundCost
	// Looking is nearly free while it keeps finding something: a round that
	// produced nothing costs four of these. Measured on a real research turn
	// that read one package for 14 rounds, learning something every round —
	// pricing looking any higher ends turns that are working.
	yieldPartial = runwayRoundCost - 1
	lookOnlyBurn = runwayRoundCost - yieldPartial

	// The tuned quantities, in look-only rounds: what a fresh turn opens with,
	// the most a productive one banks, and where the host starts saying what it
	// sees. A turn producing nothing at all burns four times as fast.
	RunwayStart = 24 * lookOnlyBurn
	runwayCap   = 40 * lookOnlyBurn
	runwayLow   = 4 * lookOnlyBurn
)

// Runway is one turn's account. State is per user turn, like the ledger whose
// rounds it settles.
type Runway struct {
	balance int
	started bool
	dry     int // consecutive rounds that produced nothing new
	idle    int // consecutive rounds that ran, changed or verified nothing
}

// RunwayState is what the host observed after settling one round. It carries
// facts only: no thresholds, no verdict about what the model should do next.
type RunwayState struct {
	Balance int
	// Rounds is how many more rounds the balance covers if the turn keeps
	// producing what it just produced; 0 while the account is growing.
	Rounds int
	Dry    int
	Idle   int
	// Spent marks the round that emptied the account — the single transition
	// where the host changes its own behavior and says so.
	Spent bool
	// Low reports a balance worth stating out loud while it drains.
	Low bool
}

// Reset opens a fresh account for a new user turn.
func (r *Runway) Reset() { *r = Runway{} }

// Settle folds one round's outcome into the account and reports what the host
// now sees.
func (r *Runway) Settle(s OutcomeSample) RunwayState {
	if !r.started {
		r.balance, r.started = RunwayStart, true
	}
	yield := roundYield(s)
	solvent := r.balance > 0
	r.balance = min(max(r.balance+yield-runwayRoundCost, 0), runwayCap)
	spent := solvent && r.balance == 0

	if s.Discriminating > 0 || s.Objective > 0 || s.Churn > 0 {
		r.idle = 0
	} else {
		r.idle++
	}
	if yield > 0 {
		r.dry = 0
	} else {
		r.dry++
	}
	// A round that earned more than it cost is refilling the account, so there
	// is no rate to project and nothing worth saying about the balance.
	burn := runwayRoundCost - yield
	state := RunwayState{Balance: r.balance, Dry: r.dry, Idle: r.idle, Spent: spent}
	if burn > 0 {
		state.Rounds = (r.balance + burn - 1) / burn
		state.Low = r.balance <= runwayLow
	}
	return state
}

// roundYield prices one round: what it bought against what every round costs.
func roundYield(s OutcomeSample) int {
	switch {
	case s.Discriminating > 0 || s.Objective > 0:
		return yieldFalsifiable
	case s.Churn > 0:
		return yieldChange
	case s.Exploration > 0:
		return yieldPartial
	default:
		return 0
	}
}
