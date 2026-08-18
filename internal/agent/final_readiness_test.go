package agent

import (
	"strings"
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/instruction"
	"reasonix/internal/provider"
	"reasonix/internal/taskpolicy"
	"reasonix/internal/tool"
)

func readinessLedger(receipts ...evidence.Receipt) *evidence.Ledger {
	l := evidence.NewLedger()
	for _, r := range receipts {
		l.Record(r)
	}
	return l
}

func TestFinalReadinessFailureBranches(t *testing.T) {
	check := instruction.VerifyCheck{Command: "go test ./...", SourcePath: "AGENTS.md", Line: 3}
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"a.go"}}
	readOnly := evidence.Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{"a.go"}}
	checkAfter := evidence.Receipt{ToolName: "bash", Success: true, Command: "go test ./..."}
	todo := evidence.Receipt{ToolName: "todo_write", Success: true, Todos: []evidence.TodoItem{{Content: "edit", Status: "in_progress"}}}
	completeAfter := evidence.Receipt{ToolName: "complete_step", Success: true, Step: "edit"}
	doneTodo := evidence.Receipt{ToolName: "todo_write", Success: true, Todos: []evidence.TodoItem{{Content: "edit", Status: "completed"}}}

	cases := []struct {
		name        string
		checks      []instruction.VerifyCheck
		evidence    *evidence.Ledger
		wantEmpty   bool
		wantContain string
	}{
		{"nil evidence never gates", []instruction.VerifyCheck{check}, nil, true, ""},
		{"no writer never gates", []instruction.VerifyCheck{check}, readinessLedger(checkAfter), true, ""},
		{"todo-only turn may end with incomplete list", nil, readinessLedger(todo), true, ""},
		{"read-only context plus todo may end with incomplete list", nil, readinessLedger(readOnly, todo), true, ""},
		{"completed todo without writer satisfies", nil, readinessLedger(doneTodo), true, ""},
		{"writer without checks or todo never gates", nil, readinessLedger(writer), true, ""},
		{"missing project check after writer is reported", []instruction.VerifyCheck{check}, readinessLedger(checkAfter, writer), false, "go test ./..."},
		{"project check run after writer satisfies", []instruction.VerifyCheck{check}, readinessLedger(writer, checkAfter), true, ""},
		{"todo writer without complete_step is reported", nil, readinessLedger(writer, todo), false, "incomplete items"},
		{"complete_step without final todo update is reported", nil, readinessLedger(writer, todo, completeAfter), false, "latest successful todo_write"},
		{"todo writer with complete_step and completed todo satisfies", nil, readinessLedger(writer, todo, completeAfter, doneTodo), true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{task: taskRuntime{ledger: tc.evidence}, projectChecks: tc.checks}
			got := a.ReadinessResult()
			if tc.wantEmpty {
				if !got.Ready {
					t.Fatalf("ReadinessResult() = %+v, want ready (no gate)", got)
				}
				return
			}
			if got.Ready {
				t.Fatalf("ReadinessResult() = ready, want a failure mentioning %q", tc.wantContain)
			}
			if !strings.Contains(got.Reason, tc.wantContain) {
				t.Fatalf("ReadinessResult().Reason = %q, want it to mention %q", got.Reason, tc.wantContain)
			}
			if len(got.Missing) == 0 {
				t.Fatalf("ReadinessResult().Missing empty for %q", tc.wantContain)
			}
		})
	}
}

func TestFinalReadinessAllowsIncompleteTodosInPlanMode(t *testing.T) {
	todo := evidence.Receipt{ToolName: "todo_write", Success: true, Todos: []evidence.TodoItem{{Content: "draft implementation plan", Status: "pending"}}}
	a := &Agent{task: taskRuntime{ledger: readinessLedger(todo)}}
	a.SetPlanMode(true)

	if got := a.ReadinessResult(); !got.Ready {
		t.Fatalf("ReadinessResult() = %+v, want ready in plan mode", got)
	}
	if got := a.finalReadinessCheckFor(); got.applies {
		t.Fatalf("finalReadinessCheckFor() applies in plan mode: %+v", got)
	}
}
func TestFinalReadinessIgnoresLoopGuardQuotedInToolOutput(t *testing.T) {
	todo := evidence.Receipt{ToolName: "todo_write", Success: true, Todos: []evidence.TodoItem{{Content: "edit", Status: "in_progress"}}}
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"a.go"}}
	sess := NewSession("")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "edit"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "b1", Name: "bash"}}})
	sess.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "b1", Name: "bash", Content: "agent.go:2082: \"[loop guard] %s has now %s %d times\""})
	a := &Agent{task: taskRuntime{ledger: readinessLedger(writer, todo)}, sess: sessionRuntime{conversation: sess}}

	if got := a.finalReadinessCheckFor(); got.reason == "" {
		t.Fatal("finalReadinessCheckFor() reason empty, want quoted loop-guard text to be ignored")
	}
}

// The table will never enumerate every project's own runner, so a project
// driven by its own scripts runs its check, passes it, and is told it verified
// nothing. That miss is the host's, so it is reported and not charged: failing
// the turn punishes work that did verify itself, and the gate lapses next turn
// regardless, so the failure collects nothing later either.
func TestVerificationGapSeparatesTheHostsBlindSpotFromTheTurns(t *testing.T) {
	const ran = "./check.sh"
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"wordcount.sh"}}
	command := evidence.Receipt{ToolName: "bash", Success: true, Command: ran}
	declared := instruction.VerifyCheck{Command: "make check", SourcePath: "AGENTS.md", Line: 3}

	cases := []struct {
		name         string
		checks       []instruction.VerifyCheck
		ledger       *evidence.Ledger
		wantAdvisory bool
		wantQuoted   bool
	}{
		{"a command the table could not read is advisory", nil, readinessLedger(writer, command), true, true},
		{"nothing ran at all is the turn's own miss", nil, readinessLedger(writer), false, false},
		{"a project that declared checks owes those", []instruction.VerifyCheck{declared}, readinessLedger(writer, command), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{task: taskRuntime{ledger: tc.ledger}, projectChecks: tc.checks}
			got, advisory := a.verificationGap(0)
			if advisory != tc.wantAdvisory {
				t.Fatalf("advisory = %v, want %v: %q", advisory, tc.wantAdvisory, got)
			}
			if strings.Contains(got, ran) != tc.wantQuoted {
				t.Errorf("quoting what ran = %v, want %v: %q", !tc.wantQuoted, tc.wantQuoted, got)
			}
			if !advisory && !strings.Contains(got, "run a relevant verification command") {
				t.Errorf("a blocking gap must still carry the ask: %q", got)
			}
			if advisory && !strings.Contains(got, instruction.HostChecksHeading) {
				t.Errorf("an advisory must say how to make the check count: %q", got)
			}
		})
	}
}

// An advisory reports; it must never be what fails a run.
func TestAdvisoryDoesNotFailTheTurn(t *testing.T) {
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"wordcount.sh"}}
	command := evidence.Receipt{ToolName: "bash", Success: true, Command: "./check.sh"}
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash"})

	a := &Agent{
		task: taskRuntime{ledger: readinessLedger(writer, command)},
		svc:  agentServices{tools: reg},
		turn: turnRuntime{policySet: true, policy: taskpolicy.TaskPolicy{Verification: taskpolicy.VerifyTargeted}},
	}
	got := a.ReadinessResult()
	if !got.Ready {
		t.Fatalf("ReadinessResult() = %+v, want ready: the host's blind spot must not fail the run", got)
	}
	if got.Advisory == "" {
		t.Error("the advisory was dropped; the user never learns the check went unread")
	}
}

// A cited check the ledger corroborates settles readiness on its own: the model
// says which command was the check, the host proves it ran and passed.
func TestCitedCheckSettlesWhatTheTableCouldNot(t *testing.T) {
	const ran = "python scripts/screening.py --self-check"
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"screening.py"}}
	command := evidence.Receipt{ToolName: "bash", Success: true, Command: ran}
	signoff := evidence.Receipt{ToolName: "complete_step", Success: true, Step: "screen", CitedChecks: []string{ran}}

	a := &Agent{task: taskRuntime{ledger: readinessLedger(writer, command, signoff)}}
	if !a.checkEstablished(0, false) {
		t.Fatal("a corroborated citation after the write must establish the check")
	}

	uncorroborated := evidence.Receipt{ToolName: "complete_step", Success: true, Step: "screen", CitedChecks: []string{"python never_ran.py"}}
	b := &Agent{task: taskRuntime{ledger: readinessLedger(writer, command, uncorroborated)}}
	if b.checkEstablished(0, false) {
		t.Fatal("a citation naming a command that never ran must not establish anything")
	}
}

// The heading the host quotes must be the heading it parses.
func TestHostChecksHeadingIsWhatTheParserReads(t *testing.T) {
	body := "## " + instruction.HostChecksHeading + "\n\n- verify: python -m pytest\n"
	checks := instruction.ExtractHostChecks([]instruction.Document{{Path: "AGENTS.md", Body: body}})
	if len(checks) != 1 || checks[0].Command != "python -m pytest" {
		t.Fatalf("ExtractHostChecks under the exported heading = %+v, want the declared check", checks)
	}
}
