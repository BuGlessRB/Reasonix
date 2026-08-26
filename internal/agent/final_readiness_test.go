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
// nothing. That miss is the host's, so the turn is not charged for it: failing
// work that did verify itself punishes the gate's blind spot, and the gate
// lapses next turn regardless, so the failure collects nothing later either.
func TestVerificationGapSeparatesTheHostsBlindSpotFromTheTurns(t *testing.T) {
	const ran = "./check.sh"
	writer := evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"wordcount.sh"}}
	command := evidence.Receipt{ToolName: "bash", Success: true, Command: ran}
	declared := instruction.VerifyCheck{Command: "make check", SourcePath: "AGENTS.md", Line: 3}

	cases := []struct {
		name     string
		checks   []instruction.VerifyCheck
		ledger   *evidence.Ledger
		wantOwed bool
	}{
		{"a command the table could not read is owed by nobody", nil, readinessLedger(writer, command), false},
		{"nothing ran at all is the turn's own miss", nil, readinessLedger(writer), true},
		{"a project that declared checks owes those", []instruction.VerifyCheck{declared}, readinessLedger(writer, command), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{task: taskRuntime{ledger: tc.ledger}, projectChecks: tc.checks}
			gap, owed := a.verificationGap(0)
			if owed != tc.wantOwed {
				t.Fatalf("owed = %v, want %v: %q", owed, tc.wantOwed, gap)
			}
			// Nothing owed means nothing said. The host has no honest sentence
			// for a command it could not read, and a gap with no ask in it was
			// only ever a flag wearing a message's type.
			if !owed && gap != "" {
				t.Errorf("a waived gap still carries a message: %q", gap)
			}
			if strings.Contains(gap, ran) {
				t.Errorf("a gap carries the command verbatim: %q", gap)
			}
			if owed && !strings.Contains(gap, "run a relevant verification command") {
				t.Errorf("an owed gap must still carry the ask: %q", gap)
			}
		})
	}
}

// The host's blind spot must never be what fails a run.
func TestUnreadableCommandDoesNotFailTheTurn(t *testing.T) {
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

// Self-inspection is per file, and fresh per file. Reading one of five was the
// old floor and it is what let a five-file change ship on one file's attention;
// reading them in edit-then-read order must still count, or a turn that
// inspected everything gets sent back because it went on to touch a fifth.
func TestSelfInspectionCoversEveryChangedFile(t *testing.T) {
	write := func(path string) evidence.Receipt {
		return evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Mutation: true,
			MutationEvidence: evidence.MutationProven, Paths: []string{path}}
	}
	read := func(path string) evidence.Receipt {
		return evidence.Receipt{ToolName: "read_file", Success: true, Read: true, Paths: []string{path}}
	}

	partial := &Agent{deliveryProfile: true, task: taskRuntime{ledger: readinessLedger(
		write("a.go"), write("b.go"), read("a.go"),
	)}}
	got := partial.finalReadinessCheckFor()
	if !strings.Contains(got.reason, "b.go") {
		t.Fatalf("reason = %q, want the uninspected file named", got.reason)
	}
	if strings.Contains(got.reason, "a.go, b.go") {
		t.Fatalf("reason = %q, must not ask again for the file it did read", got.reason)
	}

	// Edit-then-read, file by file: every read follows its own file's write.
	interleaved := &Agent{deliveryProfile: true, task: taskRuntime{ledger: readinessLedger(
		write("a.go"), read("a.go"), write("b.go"), read("b.go"),
	)}}
	if reason := interleaved.finalReadinessCheckFor().reason; strings.Contains(reason, "inspect every file") {
		t.Fatalf("reason = %q, want file-by-file inspection accepted", reason)
	}

	// A read that predates its own file's next write is stale, not coverage.
	stale := &Agent{deliveryProfile: true, task: taskRuntime{ledger: readinessLedger(
		write("a.go"), read("a.go"), write("a.go"),
	)}}
	if reason := stale.finalReadinessCheckFor().reason; !strings.Contains(reason, "a.go") {
		t.Fatalf("reason = %q, want the re-written file asked for again", reason)
	}
}
