package agent

import (
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func TestSubagentStorePartialOutcomeCanBeReadAndResumed(t *testing.T) {
	store := NewSubagentStore(t.TempDir())
	spec := testSubagentSpec(t, "explore")
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	run.Session.Add(provider.Message{Role: provider.RoleUser, Content: "inspect"})
	run.Session.Add(provider.Message{Role: provider.RoleAssistant, Content: "partial finding"})
	if err := store.SaveOutcome(run, SubagentOutcome{Ref: run.Ref, Status: SubagentOutcomePartial, FinalAnswer: "partial finding", ErrorCode: "completion_uncertain", Retryable: true}); err != nil {
		t.Fatalf("SaveOutcome: %v", err)
	}
	run.Release()
	answer, status, err := store.ReadFinalAnswer(run.Ref, spec.ParentSession, spec.WorkspaceRoot)
	if err != nil || answer != "partial finding" || status != SubagentStatus(SubagentOutcomePartial) {
		t.Fatalf("ReadFinalAnswer = %q/%q/%v, want partial result", answer, status, err)
	}
	meta, err := store.LoadMeta(run.Ref)
	if err != nil || meta.Status != SubagentFailed || meta.Outcome != string(SubagentOutcomePartial) || !meta.Retryable {
		t.Fatalf("partial metadata = %+v/%v", meta, err)
	}
	continued, err := store.PrepareContinue(run.Ref, spec)
	if err != nil {
		t.Fatalf("PrepareContinue partial: %v", err)
	}
	continued.Release()
}

func TestSubagentStoreFailedOutcomeRetainsReadableLastAnswer(t *testing.T) {
	store := NewSubagentStore(t.TempDir())
	spec := testSubagentSpec(t, "explore")
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	run.Session.Add(provider.Message{Role: provider.RoleAssistant, Content: "last useful answer"})
	if err := store.SaveOutcome(run, SubagentOutcome{Ref: run.Ref, Status: SubagentOutcomeFailed, ErrorCode: "provider_error"}); err != nil {
		t.Fatalf("SaveOutcome: %v", err)
	}
	run.Release()
	answer, status, err := store.ReadFinalAnswer(run.Ref, spec.ParentSession, spec.WorkspaceRoot)
	if err != nil || answer != "last useful answer" || status != SubagentStatus(SubagentOutcomeFailed) {
		t.Fatalf("ReadFinalAnswer = %q/%q/%v, want retained failed answer", answer, status, err)
	}
	if _, err := store.PrepareContinue(run.Ref, spec); err == nil || !strings.Contains(err.Error(), "failed and cannot be continued") {
		t.Fatalf("non-retryable failed continuation error = %v", err)
	}
}
