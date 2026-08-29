package agent

import (
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/instruction"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type projectCheckProbeSink struct {
	probes []event.ProjectCheckProbe
}

func (s *projectCheckProbeSink) Emit(event.Event) {}

func (s *projectCheckProbeSink) RecordProjectCheckProbe(p event.ProjectCheckProbe) {
	s.probes = append(s.probes, p)
}

func (s *projectCheckProbeSink) last(t *testing.T) event.ProjectCheckProbe {
	t.Helper()
	if len(s.probes) == 0 {
		t.Fatal("no project-check probe recorded")
	}
	return s.probes[len(s.probes)-1]
}

func classOf(p event.ProjectCheckProbe, identity string) string {
	for _, d := range p.Diffs {
		if d.Identity == identity {
			return d.Class
		}
	}
	return ""
}

// rewriteDeclarationRun drives a delivery turn that writes, is blocked owing
// the check the task began under, then rewrites its own declaration and runs
// the replacement instead.
func rewriteDeclarationRun(t *testing.T, began, rewrittenTo, ran string) (*projectCheckProbeSink, error) {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false, writesPaths: true})
	reg.Add(fakeTool{name: "bash", readOnly: false})
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{
			toolCallChunk("c1", "write_file", `{"path":"changed.go","content":"package main"}`),
			{Type: provider.ChunkDone},
		},
		{{Type: provider.ChunkText, Text: "premature"}, {Type: provider.ChunkDone}},
		{
			toolCallChunk("c2", "bash", `{"command":"`+ran+`"}`),
			{Type: provider.ChunkDone},
		},
		{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
	}}
	sink := &projectCheckProbeSink{}
	a := New(prov, reg, NewSession(""), Options{
		ProjectChecks: []instruction.VerifyCheck{{Command: began, SourcePath: "AGENTS.md", Line: 3}},
	}, sink)
	ctx := deliveryGoalContext("goal-probe", "edit and finish")

	if err := a.Run(ctx, "edit and finish"); !readinessBlocked(err) {
		t.Fatalf("premature Run err = %v, want FinalReadinessError", err)
	}
	// What this models is a resume, not a mid-run edit: projectChecks is loaded
	// once at boot and never reassigned, so rewriting the file inside a run
	// changes nothing. The divergence needs a process boundary — the goal's
	// checkpoint restored with the old baseline, into a build that read the new
	// declaration. Assigning the field is that state, reached the short way.
	a.projectChecks = []instruction.VerifyCheck{{Command: rewrittenTo, SourcePath: "AGENTS.md", Line: 3}}
	return sink, a.Run(ctx, "finish")
}

// A criterion the task began under cannot be retired by the declaration that
// required it changing underneath. The gate reads the declaration as it stands
// and sees nothing owed; the obligations still owe the baseline, and the probe
// says so by name. This is the mechanism, held to the state it needs; whether a
// real run reaches that state is a separate question the corpus asks.
func TestProjectCheckProbeReportsBaselinePreservation(t *testing.T) {
	const began, replacement = "go test ./...", "go test ./internal/foo"
	sink, err := rewriteDeclarationRun(t, began, replacement, replacement)

	// What the probe measures: the turn finalizes. A declaration that no longer
	// names the criterion is enough to stop being asked for it, and flipping
	// the gate to the obligations is what changes this line.
	if err != nil {
		t.Fatalf("finalization err = %v, want the rewritten declaration to ship — the gap this probe measures", err)
	}
	probe := sink.last(t)
	if probe.LegacyBlocked {
		t.Fatalf("legacy blocked on project checks = true, want false: %+v", probe)
	}
	if !probe.CandidateBlocked {
		t.Fatalf("candidate blocked = false, want the baseline criterion still owed: %+v", probe)
	}
	baseline := evidence.VerificationIdentity(began)
	if got := classOf(probe, baseline); got != event.ProjectCheckBaselinePreservation {
		t.Fatalf("class for %q = %q, want %q: %+v", baseline, got, event.ProjectCheckBaselinePreservation, probe)
	}
	if classOf(probe, evidence.VerificationIdentity(replacement)) != "" {
		t.Fatalf("the check that ran is not a disagreement: %+v", probe)
	}
}

// The other half: running the baseline does not excuse the criterion the
// project declares now. Both derivations owe it, so it is agreement, not a
// divergence — the probe must not read "candidate is stricter" into it.
func TestProjectCheckProbeAgreesOnTheNewDeclaration(t *testing.T) {
	const began, replacement = "go test ./...", "go test ./internal/foo"
	sink, _ := rewriteDeclarationRun(t, began, replacement, began)

	probe := sink.last(t)
	if !probe.LegacyBlocked || !probe.CandidateBlocked {
		t.Fatalf("both derivations must owe the current declaration: %+v", probe)
	}
	if probe.AgreedMissing != 1 || len(probe.Diffs) != 0 {
		t.Fatalf("agreed = %d diffs = %v, want 1 agreement and no divergence: %+v",
			probe.AgreedMissing, probe.Diffs, probe)
	}
}
