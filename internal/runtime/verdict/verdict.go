// Package verdict decides a turn's outcome from host evidence alone
// (docs/design/TRUSTED_EXECUTION.md §4). Evaluate has no input a model writes:
// it never reads a criterion's status, because a todo marked completed or a
// complete_step citation can set that, and a claim may trigger verification
// but never satisfy an obligation (I2). The same inputs give the same result.
package verdict

import (
	"reasonix/internal/runtime/taskcontract"
	"reasonix/internal/safety/evidence"
)

// Verdict is one obligation's state.
type Verdict string

const (
	Satisfied    Verdict = "satisfied"
	Unsatisfied  Verdict = "unsatisfied"
	Unverifiable Verdict = "unverifiable"
	Stale        Verdict = "stale"
	Owed         Verdict = "owed"
)

// Outcome is the bundle's outcome.
type Outcome string

const (
	Completed  Outcome = "completed"
	Incomplete Outcome = "incomplete"
	Failed     Outcome = "failed"
	Blocked    Outcome = "blocked"
)

// Causes an obligation is not satisfied, in the design's failure vocabulary.
const (
	CauseNoVerifier      = "verifier.none"
	CauseNotAttempted    = "evidence.missing"
	CauseStale           = "evidence.stale"
	CauseCheckFailed     = "check.failed"
	CauseHostObligation  = "obligation.outstanding"
	CauseSuppressedCheck = "verifier.suppressed"
)

// Obligation is one derived obligation and its verdict.
type Obligation struct {
	ID       string  `json:"id"`
	Source   string  `json:"source"`
	Required bool    `json:"required"`
	Verdict  Verdict `json:"verdict"`
	Cause    string  `json:"cause,omitempty"`
}

// Result is every obligation and the outcome they add up to.
type Result struct {
	Outcome     Outcome      `json:"outcome"`
	Obligations []Obligation `json:"obligations"`
}

// Input is what the host knows at seal time. Nothing in it is authored by the
// model: Receipts are host observations, Contract contributes only its
// criteria's identities and verifier shape and its checks' host-derived state,
// and HostObligations are the ledger's outstanding debts.
type Input struct {
	Contract        *taskcontract.Contract
	Receipts        []evidence.Receipt
	HostObligations []evidence.Obligation
	// Blocked is the host-checked conclusion that the task cannot be done as
	// specified.
	Blocked bool
}

// Evaluate derives every obligation's verdict and the outcome.
func Evaluate(in Input) Result {
	var obs []Obligation
	if c := in.Contract; c != nil {
		for _, req := range c.Requirements {
			obs = append(obs, criterion(req, in.Receipts))
		}
		for _, check := range c.Checks {
			obs = append(obs, checkObligation(check))
		}
	}
	for _, o := range in.HostObligations {
		obs = append(obs, Obligation{ID: o.ID, Source: "host:" + string(o.Kind), Required: true, Verdict: Owed, Cause: CauseHostObligation})
	}
	return Result{Outcome: outcomeOf(obs, in.Blocked), Obligations: obs}
}

// criterion has a host verifier only when the ask is its own evidence: an
// atomic change proven by a successful mutation. Every other criterion waits
// for a frozen verifier, which an accepted contract supplies.
func criterion(req taskcontract.Requirement, receipts []evidence.Receipt) Obligation {
	o := Obligation{ID: "criterion@" + req.ID, Source: "criterion", Required: req.Required}
	if !req.Auto || req.AutoKind != taskcontract.EvidenceMutation {
		o.Verdict, o.Cause = Unverifiable, CauseNoVerifier
		return o
	}
	for _, r := range receipts {
		if r.Success && (r.Mutation || r.Write) {
			o.Verdict = Satisfied
			return o
		}
	}
	o.Verdict, o.Cause = Owed, CauseNotAttempted
	return o
}

// checkObligation reads a check's state, which the contract derives from
// receipts alone: nothing a model calls resolves a check.
func checkObligation(check taskcontract.Check) Obligation {
	o := Obligation{ID: "check@" + check.Command, Source: "check", Required: true}
	if check.Kind == taskcontract.CheckMutation {
		o.ID = "check@mutation"
	}
	switch check.Status {
	case taskcontract.Satisfied:
		o.Verdict = Satisfied
	case taskcontract.Failed:
		o.Verdict, o.Cause = Unsatisfied, CauseCheckFailed
	case taskcontract.Stale:
		o.Verdict, o.Cause = Stale, CauseStale
	case taskcontract.Suppressed:
		o.Verdict, o.Cause = Unverifiable, CauseSuppressedCheck
	default:
		o.Verdict, o.Cause = Owed, CauseNotAttempted
	}
	return o
}

func outcomeOf(obs []Obligation, blocked bool) Outcome {
	if blocked {
		return Blocked
	}
	outcome := Completed
	for _, o := range obs {
		if !o.Required {
			continue
		}
		switch o.Verdict {
		case Satisfied:
		case Unsatisfied:
			return Failed
		default:
			outcome = Incomplete
		}
	}
	return outcome
}
