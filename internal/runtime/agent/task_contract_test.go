package agent

import (
	"encoding/json"
	"slices"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/contract"
	"reasonix/internal/runtime/plancontract"
	"reasonix/internal/safety/evidence"
	"reasonix/internal/state/instruction"
	"reasonix/internal/state/sessionstore"
	"reasonix/internal/state/trustedstate"
)

type contractAuditSink struct{ got []event.EvidenceBundleAudit }

func (s *contractAuditSink) Emit(event.Event) {}

func (s *contractAuditSink) RecordEvidenceBundle(a event.EvidenceBundleAudit) {
	s.got = append(s.got, a)
}

func (s *contractAuditSink) last(t *testing.T) event.EvidenceBundleAudit {
	t.Helper()
	if len(s.got) == 0 {
		t.Fatal("no evidence bundle sealed")
	}
	return s.got[len(s.got)-1]
}

// writeThenRun writes one file and runs command, then answers.
func writeThenRun(command string) *scriptedProvider {
	return &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{toolCallChunk("c1", "write_file", `{"path":"changed.go","content":"package main"}`), {Type: provider.ChunkDone}},
		{toolCallChunk("c2", "bash", `{"command":"`+command+`"}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
	}}
}

func contractAgent(t *testing.T, prov *scriptedProvider, declared string, store *trustedstate.Store) (*Agent, *contractAuditSink) {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false, writesPaths: true})
	reg.Add(fakeTool{name: "bash", readOnly: false})
	sink := &contractAuditSink{}
	opts := Options{EvidenceSeal: &EvidenceSeal{Store: store, Stream: "ws"}}
	if declared != "" {
		opts.ProjectChecks = []instruction.VerifyCheck{{Command: declared, SourcePath: "REASONIX.md", Line: 3}}
	}
	a := New(prov, reg, sessionstore.NewSession(""), opts, sink)
	return a, sink
}

// The check the task began under is an accepted criterion: running another
// command after the change leaves it owed, and the divergence names the
// contract rather than a host obligation.
func TestContractOwesTheCheckTheTaskBeganUnder(t *testing.T) {
	const began, ran = "go test ./...", "go vet ./..."
	store := trustedstate.Open(t.TempDir(), nil)
	a, sink := contractAgent(t, writeThenRun(ran), began, store)
	_ = a.Run(deliveryGoalContext("goal-1", "edit"), "edit")

	got := sink.last(t)
	if got.ContractRevision != 1 || got.ContractDecision != string(contract.Accepted) || got.Outcome == "completed" {
		t.Fatalf("audit = %+v, want revision 1 accepted and the turn not completed", got)
	}
	if v := sealedVerdict(t, store, got.Record, "contract@command@"+evidence.VerificationIdentity(began)); v != "owed" {
		t.Fatalf("frozen criterion verdict = %q, want owed", v)
	}
	c, err := contract.Load(store, a.DeliveryCheckpoint().Contract)
	if err != nil {
		t.Fatal(err)
	}
	want := contract.Derive(contract.Sources{Checks: []string{evidence.VerificationIdentity(began)}})
	if !slices.EqualFunc(c.Criteria, want, contract.Criterion.Equal) {
		t.Fatalf("criteria = %+v, want %+v", c.Criteria, want)
	}
}

func TestContractCriterionIsSatisfiedByItsOwnCheck(t *testing.T) {
	const began = "go test ./..."
	store := trustedstate.Open(t.TempDir(), nil)
	a, sink := contractAgent(t, writeThenRun(began), began, store)
	_ = a.Run(deliveryGoalContext("goal-1", "edit"), "edit")
	if v := sealedVerdict(t, store, sink.last(t).Record, "contract@command@"+evidence.VerificationIdentity(began)); v != "satisfied" {
		t.Fatalf("frozen criterion verdict = %q, want satisfied by the check it names", v)
	}
}

// sealedVerdict reads one obligation's verdict out of a sealed bundle.
func sealedVerdict(t *testing.T, store *trustedstate.Store, record, id string) string {
	t.Helper()
	rec, err := store.Record(trustedstate.Digest(record))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := store.Object(rec.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Verdict struct {
			Obligations []struct {
				ID      string `json:"id"`
				Verdict string `json:"verdict"`
			} `json:"obligations"`
		} `json:"verdict"`
	}
	if err := json.Unmarshal(payload, &b); err != nil {
		t.Fatal(err)
	}
	for _, o := range b.Verdict.Obligations {
		if o.ID == id {
			return o.Verdict
		}
	}
	t.Fatalf("bundle has no obligation %q: %s", id, payload)
	return ""
}

// Mechanism given a state: this test edits the checkpoint by hand, which
// nothing in production does — the file tools refuse session stores. What it
// shows is that the checkpoint is not the authority: the revision it names is,
// and host policy refuses the derivation that would drop its criterion.
func TestContractRefusesADerivationThatDropsACriterion(t *testing.T) {
	const began, other = "go test ./...", "go vet ./..."
	store := trustedstate.Open(t.TempDir(), nil)
	a, _ := contractAgent(t, writeThenRun(began), began, store)
	ctx := deliveryGoalContext("goal-1", "edit")
	_ = a.Run(ctx, "edit")
	cp := a.DeliveryCheckpoint()
	first := cp.Contract

	cp.BaselineChecks = []string{evidence.VerificationIdentity(other)}
	b, sink := contractAgent(t, writeThenRun(other), other, store)
	b.RestoreDeliveryCheckpoint(cp)
	_ = b.Run(ctx, "continue")

	got := sink.last(t)
	if got.ContractDecision != string(contract.RelaxationRefused) || got.ContractRevision != 1 {
		t.Fatalf("audit = %+v, want the derivation refused and revision 1 kept", got)
	}
	if b.DeliveryCheckpoint().Contract != first {
		t.Fatal("the checkpoint now names another revision")
	}
	if got.Outcome == "completed" {
		t.Fatalf("audit = %+v, want the criterion the edit dropped still owed", got)
	}
}

func TestContractLoadFailureIsNotReplaced(t *testing.T) {
	a, sink := contractAgent(t, writeThenRun("go test ./..."), "go test ./...", trustedstate.Open(t.TempDir(), nil))
	a.RestoreDeliveryCheckpoint(evidence.DeliveryCheckpoint{ScopeID: "goal-1", Contract: "sha256:" + string(make([]byte, 0))})
	_ = a.Run(deliveryGoalContext("goal-1", "edit"), "edit")
	got := sink.last(t)
	if got.ContractFailure == "" || got.ContractRevision != 0 {
		t.Fatalf("audit = %+v, want a failure and no revision accepted in its place", got)
	}
}

func verifiedPlan() plancontract.Plan {
	return plancontract.Plan{
		Objective: "fix the parser",
		Steps: []plancontract.Step{
			{
				ID: "p1", Title: "fix it",
				Acceptance: []plancontract.Criterion{
					{Text: "quoted fields parse"},
					{Text: "timing is logged", Optional: true},
				},
				Verification: []plancontract.Verification{{Command: "go test ./parser/"}},
			},
			{ID: "p2", Title: "document it", Acceptance: []plancontract.Criterion{{Text: "the README says so"}}},
		},
	}.Normalize()
}

// An approved plan's criterion is verified by the commands its step names:
// they came with the plan the user approved, so the host runs them against the
// ledger instead of taking a citation on the model's word.
func TestPlanCriterionIsVerifiedByItsStepsCommands(t *testing.T) {
	plan := verifiedPlan()
	quoted, readme := plan.Steps[0].Acceptance[0].ID, plan.Steps[1].Acceptance[0].ID
	for _, tc := range []struct {
		ran  string
		want string
	}{{"go test ./parser/", "satisfied"}, {"go vet ./...", "owed"}} {
		store := trustedstate.Open(t.TempDir(), nil)
		a, sink := contractAgent(t, writeThenRun(tc.ran), "", store)
		a.SetPlanContract(&plan)
		_ = a.Run(deliveryGoalContext("goal-1", "fix"), "fix")
		record := sink.last(t).Record
		if v := sealedVerdict(t, store, record, "contract@plan@"+quoted); v != tc.want {
			t.Fatalf("after %q the plan criterion is %q, want %q", tc.ran, v, tc.want)
		}
		if v := sealedVerdict(t, store, record, "contract@plan@"+readme); v != "unverifiable" {
			t.Fatalf("a criterion whose step names no command is %q, want unverifiable", v)
		}
		if hasObligation(t, store, record, "criterion@"+quoted) {
			t.Fatal("the plan criterion is counted twice: once frozen, once from the replayed contract")
		}
	}
}

func hasObligation(t *testing.T, store *trustedstate.Store, record, id string) bool {
	t.Helper()
	rec, err := store.Record(trustedstate.Digest(record))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := store.Object(rec.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Verdict struct {
			Obligations []struct {
				ID string `json:"id"`
			} `json:"obligations"`
		} `json:"verdict"`
	}
	if err := json.Unmarshal(payload, &b); err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(b.Verdict.Obligations, func(o struct {
		ID string `json:"id"`
	}) bool {
		return o.ID == id
	})
}
