package evidence

import "testing"

func TestVerificationExitConclusive(t *testing.T) {
	cases := []struct {
		command    string
		conclusive bool
	}{
		{"go test ./...", true},
		{"go test ./... 2>&1", true},
		{"cd /w && go vet ./... && go test ./...", true},
		// `head` reports the status, and it succeeds on a failing suite.
		{"go test ./... 2>&1 | head -100", false},
		// `git status` reports the status, and it fails outside a repository.
		{"go test ./... | tail -20 && git diff --stat; git status --short", false},
		{"go test ./... || echo done", false},
		{"go test ./... &", false},
		// The suite reports the status even though a `;` precedes it.
		{`git status --short 2>/dev/null || echo "no repo"; go test ./... 2>&1`, true},
		// A trailing reporter takes the status back over.
		{"go test ./...; echo $?", false},
		// `&&` short-circuits, so a zero status still means the suite passed.
		{"go test ./... && echo done", true},
		// The suite is still short-circuited even when a later stage is piped.
		{"go test ./... && go vet ./... 2>&1 | head -20", true},
		{"go test ./... || go test ./pkg", false},
	}
	for _, tc := range cases {
		if got := VerificationExitConclusive(tc.command); got != tc.conclusive {
			t.Errorf("VerificationExitConclusive(%q) = %v, want %v", tc.command, got, tc.conclusive)
		}
	}
}

func TestVerificationIdentityIgnoresTheWrapper(t *testing.T) {
	same := []string{
		"go test ./...",
		"go test ./... 2>&1",
		"cd /w && go test ./...",
		"cd /w && ls -la && go test ./... 2>&1 | head -100",
	}
	want := VerificationIdentity(same[0])
	for _, command := range same[1:] {
		if got := VerificationIdentity(command); got != want {
			t.Errorf("VerificationIdentity(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestVerificationIdentitySeparatesDifferentChecks(t *testing.T) {
	if VerificationIdentity("go test ./...") == VerificationIdentity("go vet ./...") {
		t.Fatal("VerificationIdentity: distinct checks must not share an identity")
	}
}

// A command no static pass can decompose keeps its own text, so two unrelated
// unparseable commands never merge into one item.
func TestVerificationIdentityFallsBackToTheCommand(t *testing.T) {
	command := "go test ./... $(cat"
	if got := VerificationIdentity(command); got != command {
		t.Fatalf("VerificationIdentity(%q) = %q, want the command itself", command, got)
	}
}

// The post-change verification floor must not be satisfied by a suite whose
// verdict the shell hid: that is exactly the shape that exits 0 while red.
func TestVerificationFloorRejectsAnUnreadableRun(t *testing.T) {
	piped := NewLedger()
	piped.Record(Receipt{
		ToolName:     "bash",
		Command:      "go test ./... | head -100",
		Success:      true,
		Verification: VerificationInconclusive,
	})
	if piped.HasSuccessfulVerificationCommandAfter(-1) {
		t.Fatal("a piped suite satisfied the verification floor; its exit status proves nothing")
	}

	clean := NewLedger()
	clean.Record(Receipt{
		ToolName:     "bash",
		Command:      "go test ./...",
		Success:      true,
		Verification: VerificationPassed,
	})
	if !clean.HasSuccessfulVerificationCommandAfter(-1) {
		t.Fatal("a readable passing run did not satisfy the verification floor")
	}
}

// A check is not discarded for its company: the host cannot prove `gofmt -l .`
// leaves the tree alone, but `&&` still proves the suite that ran before it
// passed. Delivery sign-off keeps asking the stricter question.
func TestVerificationIsNotDiscardedForItsCompany(t *testing.T) {
	const mixed = "go test ./... && go vet ./... && gofmt -l ."
	if !CommandRunsVerification(mixed) {
		t.Fatal("a suite in a chain with an unclassified command stopped counting as verification")
	}
	if !VerificationExitConclusive(mixed) {
		t.Fatal("an && chain hides no failure, so the suite's verdict is readable")
	}
	if IsDeliveryVerificationCommand(mixed) {
		t.Fatal("delivery sign-off must keep rejecting a command it cannot prove read-only")
	}
}

// The company must not launder an unreadable verdict either: here the suite is
// piped and a later stage owns the status, so nothing is proven.
func TestCompanyDoesNotLaunderAPipedVerdict(t *testing.T) {
	const laundered = "go test ./... | head -50 && gofmt -l ."
	if !CommandRunsVerification(laundered) {
		t.Fatal("the command still runs a verification and must carry a verdict")
	}
	if VerificationExitConclusive(laundered) {
		t.Fatal("a piped suite proves nothing even when a later stage succeeds")
	}
}

// Receipts from before host classification existed keep their old meaning.
func TestVerificationFloorKeepsUnclassifiedReceipts(t *testing.T) {
	ledger := NewLedger()
	ledger.Record(Receipt{ToolName: "bash", Command: "go test ./...", Success: true})
	if !ledger.HasSuccessfulVerificationCommandAfter(-1) {
		t.Fatal("an unclassified successful run must keep counting as verification")
	}
}
