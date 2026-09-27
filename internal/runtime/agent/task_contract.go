package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"

	"reasonix/internal/runtime/contract"
	"reasonix/internal/runtime/verdict"
	"reasonix/internal/safety/evidence"
	"reasonix/internal/state/trustedstate"
)

// contractState is what the bundle records about the task's contract.
type contractState struct {
	Record   string               `json:"record,omitempty"`
	Revision int                  `json:"revision,omitempty"`
	Decision contract.Decision    `json:"decision,omitempty"`
	Failure  string               `json:"failure,omitempty"`
	Criteria []contract.Criterion `json:"criteria,omitempty"`
}

// settleContract applies host policy to the criteria the task's host-owned
// sources derive now, against the revision in force. A revision the checkpoint
// names but the store cannot produce is a failure, never a reason to accept a
// fresh one in its place: that would invent an acceptance nobody made.
func (a *Agent) settleContract(ctx context.Context, seal *EvidenceSeal) contractState {
	cur, record, err := a.contractInForce(seal)
	if err != nil {
		return contractState{Failure: trustedstate.FailureCode(err)}
	}
	derived := contract.Derive(contract.Sources{Checks: a.task.checkpoint.BaselineChecks, Tests: a.baselineTestIdentities(), Plan: a.planCriteria()})
	next, decision := contract.Accept(cur, record, derived, a.contractID())
	if decision == contract.Accepted || decision == contract.Tightened {
		rec, err := contract.Seal(ctx, seal.Store, seal.Stream, next)
		if err != nil {
			st := contractState{Failure: trustedstate.FailureCode(err)}
			if cur != nil {
				st.Record, st.Revision, st.Criteria = record, cur.Revision, cur.Criteria
			}
			return st
		}
		a.task.contract, a.task.contractRecord = &next, rec
		a.task.checkpoint.Contract = rec
		record = rec
	}
	return contractState{Record: record, Revision: next.Revision, Decision: decision, Criteria: next.Criteria}
}

// contractInForce is the revision the checkpoint names. The cached copy is
// trusted only while it is the one named, so a checkpoint reset for a new scope
// starts a new contract rather than inheriting the last one.
func (a *Agent) contractInForce(seal *EvidenceSeal) (*contract.Contract, string, error) {
	named := a.task.checkpoint.Contract
	if named == "" {
		return nil, "", nil
	}
	if a.task.contract != nil && a.task.contractRecord == named {
		return a.task.contract, named, nil
	}
	c, err := contract.Load(seal.Store, named)
	if err != nil {
		return nil, "", err
	}
	a.task.contract, a.task.contractRecord = &c, named
	return &c, named, nil
}

func (a *Agent) contractID() string {
	if id := a.task.checkpoint.ScopeID; id != "" {
		return id
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "task-" + hex.EncodeToString(b[:])
}

func (a *Agent) baselineTestIdentities() []string {
	out := make([]string, 0, len(a.task.baselineCriteria))
	for _, c := range a.task.baselineCriteria {
		out = append(out, c.Identity())
	}
	return out
}

// frozenResults answers each accepted criterion from the ledger, never from
// the checkpoint's copy of the declaration: that copy lives outside Trusted
// Host State, so reading it would let an edit there speak for the contract.
func (a *Agent) frozenResults(criteria []contract.Criterion) []verdict.Frozen {
	if len(criteria) == 0 {
		return nil
	}
	ledger := a.task.ledger
	at, changed := ledger.LatestSuccessfulMutationIndex()
	owedTests := map[string]bool{}
	for _, o := range evidence.BaselineTestObligations(a.baselineFacts(), a.mutationEpoch()) {
		owedTests[o.ID] = true
	}
	known := map[string]bool{}
	for _, id := range a.baselineTestIdentities() {
		known[id] = true
	}
	out := make([]verdict.Frozen, 0, len(criteria))
	for _, c := range criteria {
		f := verdict.Frozen{ID: c.ID, Source: c.Source, Identity: c.Verifier.Identity}
		switch c.Verifier.Kind {
		case contract.VerifierCommand:
			f.Satisfied = !changed || ledger.HasSuccessfulCommandAfter(c.Verifier.Identity, at)
		case contract.VerifierTest:
			f.Unverifiable = !known[c.Verifier.Identity]
			f.Satisfied = !f.Unverifiable && !owedTests["baseline_test@"+c.Verifier.Identity]
		case contract.VerifierCommands:
			f.Satisfied = !changed || !slices.ContainsFunc(c.Verifier.Identities, func(id string) bool {
				return !ledger.HasSuccessfulCommandAfter(id, at)
			})
		default:
			f.Unverifiable, f.NoVerifier = true, true
		}
		out = append(out, f)
	}
	return out
}

// planCriteria are the approved plan's acceptance criteria, each verified by
// the verification commands its step names. The user approved those commands
// with the plan, so they are the plan's own verifiers rather than the model's.
func (a *Agent) planCriteria() []contract.PlanCriterion {
	plan := a.PlanContract()
	if plan == nil {
		return nil
	}
	var out []contract.PlanCriterion
	for _, step := range plan.Steps {
		var commands []string
		for _, v := range step.Verification {
			if id := evidence.VerificationIdentity(strings.TrimSpace(v.Command)); id != "" {
				commands = append(commands, id)
			}
		}
		for _, c := range step.Acceptance {
			out = append(out, contract.PlanCriterion{ID: c.ID, Required: !c.Optional, Commands: commands})
		}
	}
	return out
}
