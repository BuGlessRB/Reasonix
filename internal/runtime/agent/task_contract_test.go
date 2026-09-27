package agent

import (
	"encoding/json"
	"slices"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/contract"
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
	a := New(prov, reg, sessionstore.NewSession(""), Options{
		ProjectChecks: []instruction.VerifyCheck{{Command: declared, SourcePath: "REASONIX.md", Line: 3}},
		EvidenceSeal:  &EvidenceSeal{Store: store, Stream: "ws"},
	}, sink)
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
	if !slices.Equal(c.Criteria, want) {
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
