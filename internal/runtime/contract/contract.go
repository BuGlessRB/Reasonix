package contract

import (
	"encoding/json"
	"slices"
	"strings"

	"reasonix/internal/state/trustedstate"
)

// Verifier kinds.
const (
	VerifierCommand  = "command"
	VerifierTest     = "test"
	VerifierCommands = "commands"
	VerifierNone     = "none"
)

// Verifier is what satisfies a criterion: a verification command recognised
// by its canonical identity, a captured test criterion by its identity, every
// command of a plan step by theirs, or nothing the host can run.
type Verifier struct {
	Kind       string   `json:"kind"`
	Identity   string   `json:"identity,omitempty"`
	Identities []string `json:"identities,omitempty"`
}

// Criterion is one accepted acceptance criterion.
type Criterion struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Required bool     `json:"required"`
	Verifier Verifier `json:"verifier"`
}

// Contract is one accepted revision.
type Contract struct {
	ID         string      `json:"id"`
	Revision   int         `json:"revision"`
	Parent     string      `json:"parent,omitempty"`
	Criteria   []Criterion `json:"criteria"`
	AcceptedBy string      `json:"accepted_by"`
}

// Sources are the host-owned inputs a revision is derived from.
type Sources struct {
	// Checks are canonical identities of the check commands the task began
	// requiring (evidence.CheckContract baseline).
	Checks []string
	// Tests are captured test criterion identities.
	Tests []string
	// Plan are the approved plan's acceptance criteria, each verified by the
	// verification commands of the step it belongs to.
	Plan []PlanCriterion
}

// PlanCriterion is one plan acceptance criterion and the canonical identities
// of its step's verification commands; none means the host cannot check it.
type PlanCriterion struct {
	ID       string
	Required bool
	Commands []string
}

// Sources a criterion came from.
const (
	SourceProjectCheck = "project_check"
	SourceBaselineTest = "baseline_test"
	SourcePlan         = "plan"
)

// PolicyTemplate names the host policy that accepts a derived revision.
const PolicyTemplate = "host_policy:template/1"

// Derive turns sources into criteria in a canonical order, so the same sources
// always derive byte-identical criteria.
func Derive(s Sources) []Criterion {
	var out []Criterion
	add := func(source, kind string, ids []string) {
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			c := Criterion{ID: kind + "@" + id, Source: source, Required: true, Verifier: Verifier{Kind: kind, Identity: id}}
			if !slices.ContainsFunc(out, func(x Criterion) bool { return x.ID == c.ID }) {
				out = append(out, c)
			}
		}
	}
	add(SourceProjectCheck, VerifierCommand, s.Checks)
	add(SourceBaselineTest, VerifierTest, s.Tests)
	for _, p := range s.Plan {
		id := strings.TrimSpace(p.ID)
		if id == "" || slices.ContainsFunc(out, func(x Criterion) bool { return x.ID == "plan@"+id }) {
			continue
		}
		v := Verifier{Kind: VerifierNone}
		if ids := canonicalIDs(p.Commands); len(ids) > 0 {
			v = Verifier{Kind: VerifierCommands, Identities: ids}
		}
		out = append(out, Criterion{ID: "plan@" + id, Source: SourcePlan, Required: p.Required, Verifier: v})
	}
	slices.SortFunc(out, func(a, b Criterion) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Decision is what host policy did with a derivation.
type Decision string

const (
	// Accepted: the first revision of a new contract.
	Accepted Decision = "accepted"
	// Tightened: a new revision that keeps every criterion and adds some.
	Tightened Decision = "tightened"
	// Unchanged: the derivation matches the current revision.
	Unchanged Decision = "unchanged"
	// RelaxationRefused: the derivation drops or alters a criterion, which only
	// the User may accept; the current revision stands.
	RelaxationRefused Decision = "relaxation_refused"
)

// Accept applies host policy to derived criteria against the current revision
// (nil for a task with no contract yet). It returns the revision now in force
// and whether it is new. id names a first revision's contract.
func Accept(current *Contract, parentDigest string, derived []Criterion, id string) (Contract, Decision) {
	if current == nil {
		return Contract{ID: id, Revision: 1, Criteria: derived, AcceptedBy: PolicyTemplate}, Accepted
	}
	for _, c := range current.Criteria {
		if !slices.ContainsFunc(derived, func(d Criterion) bool { return d.Equal(c) }) {
			return *current, RelaxationRefused
		}
	}
	if len(derived) == len(current.Criteria) {
		return *current, Unchanged
	}
	return Contract{
		ID:         current.ID,
		Revision:   current.Revision + 1,
		Parent:     parentDigest,
		Criteria:   derived,
		AcceptedBy: PolicyTemplate,
	}, Tightened
}

// Encode is a revision's canonical bytes.
func (c Contract) Encode() []byte {
	b, _ := json.Marshal(c)
	return b
}

// Digest identifies a revision's exact content.
func (c Contract) Digest() string { return string(trustedstate.DigestOf(c.Encode())) }

func canonicalIDs(in []string) []string {
	var out []string
	for _, id := range in {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// Equal reports whether two criteria demand the same thing of the same verifier.
func (c Criterion) Equal(o Criterion) bool {
	return c.ID == o.ID && c.Source == o.Source && c.Required == o.Required &&
		c.Verifier.Kind == o.Verifier.Kind && c.Verifier.Identity == o.Verifier.Identity &&
		slices.Equal(c.Verifier.Identities, o.Verifier.Identities)
}
