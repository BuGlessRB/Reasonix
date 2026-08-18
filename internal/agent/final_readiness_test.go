package agent

import (
	"strings"
	"testing"

	"reasonix/internal/evidence"
	"reasonix/internal/instruction"
	"reasonix/internal/provider"
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

// The classifier recognises a fixed set of commands, so a project driven by its
// own scripts can run its checks all day and still be told to run one. Widening
// the set would read a deploy as a check; naming them is the way out, and the
// ask is the only place the model could learn that.
func TestVerificationGapNamesTheWayOutWhenNothingIsRecognised(t *testing.T) {
	const ran = "python scripts/inventory_analysis_screening.py --self-check"
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"screening.py"}}
	command := evidence.Receipt{ToolName: "bash", Success: true, Command: ran}
	declared := instruction.VerifyCheck{Command: "make check", SourcePath: "AGENTS.md", Line: 3}

	cases := []struct {
		name        string
		checks      []instruction.VerifyCheck
		ledger      *evidence.Ledger
		wantCommand bool
		wantHeading bool
	}{
		{"unrecognised command names the section", nil, readinessLedger(writer, command), true, true},
		{"nothing ran keeps the bare ask", nil, readinessLedger(writer), false, false},
		{"a project that declared checks is told about those", []instruction.VerifyCheck{declared}, readinessLedger(writer, command), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{task: taskRuntime{ledger: tc.ledger}, projectChecks: tc.checks}
			got := a.verificationGap(0)
			if !strings.Contains(got, "run a relevant verification command") {
				t.Fatalf("gap dropped the underlying ask: %q", got)
			}
			if strings.Contains(got, ran) != tc.wantCommand {
				t.Errorf("quoting the command that ran = %v, want %v: %q", !tc.wantCommand, tc.wantCommand, got)
			}
			if strings.Contains(got, instruction.HostChecksHeading) != tc.wantHeading {
				t.Errorf("naming %q = %v, want %v: %q", instruction.HostChecksHeading, !tc.wantHeading, tc.wantHeading, got)
			}
		})
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
