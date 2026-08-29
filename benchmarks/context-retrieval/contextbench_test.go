package main

import (
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

// The scorer decides what the pilot concludes, so it is held to the same rule
// the corpus is: judgements read structure. A model that says it will search
// has not searched; a recall call carrying a query has.

const scorerTarget = 42

func recallCall(id, args string) provider.Message {
	return provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
		{ID: id, Name: "recall", Arguments: args},
	}}
}

func toolResult(id, text string) provider.Message {
	return provider.Message{Role: provider.RoleTool, ToolCallID: id, Name: "recall", Content: text}
}

func hitBlock(positions ...int) string {
	var b strings.Builder
	b.WriteString("Folded context matching \"x\":\n")
	for _, p := range positions {
		fmt.Fprintf(&b, "\n#%d assistant_text\nsome snippet here\n", p)
	}
	return b.String()
}

func scorerTask() contextTask {
	return contextTask{ID: "t", AnswerMarkers: []string{"cobalt-lark-17"}}
}

func score(t *testing.T, msgs []provider.Message, final string) contextMetrics {
	t.Helper()
	m := scoreRun(msgs, scorerTask(), scorerTarget, "arm", false)
	m.scoreAnswer(final, scorerTask())
	return m
}

// The whole funnel, walked end to end.
func TestScorerReadsTheFullRetrievalChain(t *testing.T) {
	m := score(t, []provider.Message{
		recallCall("c1", `{"query":"retry boundary"}`),
		toolResult("c1", hitBlock(7, scorerTarget)),
		recallCall("c2", fmt.Sprintf(`{"positions":[%d]}`, scorerTarget)),
		toolResult("c2", "#42\n[assistant]\nthe ownership token is cobalt-lark-17"),
		{Role: provider.RoleAssistant, Content: "The token was cobalt-lark-17."},
	}, "The token was cobalt-lark-17.")

	if m.SearchCalls != 1 || m.ReadCalls != 1 {
		t.Errorf("calls = %d search / %d read, want 1/1", m.SearchCalls, m.ReadCalls)
	}
	if m.TargetSearchHits != 1 || m.FirstTargetRank != 2 {
		t.Errorf("target hit %d at rank %d, want 1 at rank 2", m.TargetSearchHits, m.FirstTargetRank)
	}
	if !m.TargetRead || !m.ReadAfterHit || m.DirectRead {
		t.Errorf("read flags = read %v afterHit %v direct %v", m.TargetRead, m.ReadAfterHit, m.DirectRead)
	}
	if !m.AnswerRecovered || m.FailureStage != stageRecovered {
		t.Errorf("stage = %q recovered=%v, want Recovered", m.FailureStage, m.AnswerRecovered)
	}
	if m.RecallReturnedTokens == 0 {
		t.Error("recall returned tokens were not counted")
	}
}

// An index arm's claim: the address was already on screen, so no search was
// needed. That is a different outcome from finding it, and gets its own stage.
func TestScorerSeparatesDirectReadFromSearch(t *testing.T) {
	m := score(t, []provider.Message{
		recallCall("c1", fmt.Sprintf(`{"positions":[%d]}`, scorerTarget)),
		toolResult("c1", "#42\nthe ownership token is cobalt-lark-17"),
		{Role: provider.RoleAssistant, Content: "cobalt-lark-17"},
	}, "cobalt-lark-17")

	if m.SearchCalls != 0 || !m.DirectRead {
		t.Errorf("search=%d direct=%v, want 0 and true", m.SearchCalls, m.DirectRead)
	}
	if m.FailureStage != stageDirectRead {
		t.Errorf("stage = %q, want %q", m.FailureStage, stageDirectRead)
	}
}

// Every way the chain can break gets its own label, so a failed run says where
// rather than only that.
func TestScorerNamesWhereTheChainBroke(t *testing.T) {
	for _, tc := range []struct {
		name  string
		msgs  []provider.Message
		final string
		want  string
	}{
		{
			name:  "never touched recall",
			msgs:  []provider.Message{{Role: provider.RoleAssistant, Content: "I do not recall that."}},
			final: "I do not recall that.",
			want:  stageNoRetrieval,
		},
		{
			name: "searched and missed",
			msgs: []provider.Message{
				recallCall("c1", `{"query":"unrelated words"}`),
				toolResult("c1", hitBlock(3, 9)),
			},
			final: "not found",
			want:  stageSearchMiss,
		},
		{
			name: "found it and never opened it",
			msgs: []provider.Message{
				recallCall("c1", `{"query":"retry boundary"}`),
				toolResult("c1", hitBlock(scorerTarget)),
			},
			final: "probably something about retries",
			want:  stageHitNotRead,
		},
		{
			name: "read it and answered wrong",
			msgs: []provider.Message{
				recallCall("c1", fmt.Sprintf(`{"positions":[%d]}`, scorerTarget)),
				toolResult("c1", "#42\nthe token is elsewhere"),
			},
			final: "I think it was amber-9",
			want:  stageAnswerWrong,
		},
		{
			name: "read the wrong position",
			msgs: []provider.Message{
				recallCall("c1", `{"positions":[7]}`),
				toolResult("c1", "#7\nnothing useful"),
			},
			final: "unclear",
			want:  stageReadMissed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := score(t, tc.msgs, tc.final).FailureStage; got != tc.want {
				t.Errorf("stage = %q, want %q", got, tc.want)
			}
		})
	}
}

// The answer lives only in folded history, so reaching for the workspace is a
// strategy change worth seeing — including when it happens to work.
func TestScorerRecordsWorkspaceEscapes(t *testing.T) {
	m := score(t, []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "g1", Name: "bash", Arguments: `{"command":"rg cobalt-lark"}`},
			{ID: "g2", Name: "read_file", Arguments: `{"path":"notes.md"}`},
		}},
		{Role: provider.RoleAssistant, Content: "cobalt-lark-17"},
	}, "cobalt-lark-17")

	if len(m.UnexpectedWorkTools) != 2 {
		t.Fatalf("escapes = %v, want bash and read_file", m.UnexpectedWorkTools)
	}
	if m.SearchCalls != 0 || m.ReadCalls != 0 {
		t.Errorf("a workspace tool was counted as recall: search=%d read=%d", m.SearchCalls, m.ReadCalls)
	}
	// It answered, so the run is Recovered — the escape is reported beside the
	// outcome rather than overriding it.
	if !m.AnswerRecovered {
		t.Error("a correct answer was not scored as recovered")
	}
}

// Narration is not action.
func TestScorerIgnoresWhatTheModelSaysItWillDo(t *testing.T) {
	m := score(t, []provider.Message{
		{Role: provider.RoleAssistant, Content: "Let me search the folded context with recall for the retry boundary."},
	}, "Let me search the folded context with recall for the retry boundary.")
	if m.SearchCalls != 0 || m.FailureStage != stageNoRetrieval {
		t.Errorf("prose was scored as a search: calls=%d stage=%q", m.SearchCalls, m.FailureStage)
	}
}

// Every marker must land, so a partially right answer is not a recovery.
func TestScorerRequiresEveryMarker(t *testing.T) {
	task := contextTask{ID: "t", AnswerMarkers: []string{"comet-42", "per-lineage"}}
	m := scoreRun(nil, task, scorerTarget, "arm", false)
	m.scoreAnswer("the salt was comet-42", task)
	if m.AnswerRecovered {
		t.Error("a half answer scored as recovered")
	}
	if len(m.MissingMarkers) != 1 || m.MissingMarkers[0] != "per-lineage" {
		t.Errorf("missing = %v, want per-lineage", m.MissingMarkers)
	}
}
