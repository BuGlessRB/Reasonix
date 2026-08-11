package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// grepProbe mirrors the real grep tool's receipt shape: read-only, path-scoped,
// and carrying its question in an argument rather than in the path.
type grepProbe struct{}

func (grepProbe) Name() string        { return "grep" }
func (grepProbe) Description() string { return "search" }
func (grepProbe) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`)
}
func (grepProbe) ReadOnly() bool { return true }
func (grepProbe) Execute(context.Context, json.RawMessage) (string, error) {
	return "internal/agent/agent.go:120: match\ninternal/agent/task.go:44: match\n", nil
}

type roundCall struct {
	name string
	args string
}

// runProbeRounds executes one call per round and returns each round's host
// observation, indexed from round 1.
func runProbeRounds(t *testing.T, rounds []roundCall) []string {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(readProbe{})
	reg.Add(grepProbe{})
	reg.Add(fakeTool{name: "bash", readOnly: false})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)
	a.resetTurnEvidence()

	said := make([]string, len(rounds)+1)
	for i, rc := range rounds {
		batch := a.executeBatch(context.Background(), []provider.ToolCall{
			{ID: fmt.Sprintf("c%d", i), Name: rc.name, Arguments: rc.args},
		})
		if out := batch.results[0]; strings.Contains(out, "[host]") {
			said[i+1] = out[strings.Index(out, "[host]"):]
		}
	}
	return said
}

func assertNeverCalledRepetitive(t *testing.T, said []string) {
	t.Helper()
	for round, text := range said {
		if strings.Contains(text, "produced nothing new") {
			t.Errorf("round %d was told it produced nothing new, but every round asked something new: %q", round, text)
		}
	}
}

// A research turn reading a file it has never read before, every round. The old
// ladder called this "no new evidence" and demanded a final answer at round 12;
// the account prices it as real but slow progress, and the only thing the host
// eventually says is what it measured.
func TestReadingNewFilesIsPricedAsProgressNotRepetition(t *testing.T) {
	var rounds []roundCall
	for i := 1; i <= evidence.RunwayStart+2; i++ {
		rounds = append(rounds, roundCall{"read_file", fmt.Sprintf(`{"path":"internal/pkg%d/file.go"}`, i)})
	}
	said := runProbeRounds(t, rounds)
	assertNeverCalledRepetitive(t, said)

	spent := 0
	for round, text := range said {
		if strings.Contains(text, "runway is spent") {
			spent = round
			break
		}
	}
	t.Logf("a look-only turn reached the end of its runway at round %d", spent)
	if spent == 0 {
		t.Fatal("a turn that never runs or changes anything must still reach the end of its runway")
	}
	if spent <= evidence.RunwayStart/2 {
		t.Errorf("runway ran out at round %d; looking must buy back most of its own cost", spent)
	}
}

// Searching one package with a different pattern each round is the standard
// investigation move. Keying novelty on the path alone scored every round after
// the first as a repeat and stopped the turn at round 7.
func TestGrepSamePathNewPatternsIsNotARepeat(t *testing.T) {
	var rounds []roundCall
	for _, p := range []string{"Compose", "Receipt", "Ledger", "progressGuard", "applyEBM", "Delivery", "ToolCall", "Session", "compact", "readiness"} {
		rounds = append(rounds, roundCall{"grep", fmt.Sprintf(`{"pattern":%q,"path":"internal/agent"}`, p)})
	}
	said := runProbeRounds(t, rounds)
	assertNeverCalledRepetitive(t, said)
	for round, text := range said {
		if strings.Contains(text, "runway is spent") {
			t.Errorf("round %d ended the turn after %d distinct searches", round, len(rounds))
		}
	}
}

// Paging through one long file: distinct windows, one path.
func TestPagingOneFileIsNotARepeat(t *testing.T) {
	var rounds []roundCall
	for i := range 10 {
		rounds = append(rounds, roundCall{"read_file", fmt.Sprintf(`{"path":"internal/agent/agent.go","offset":%d,"limit":300}`, i*300)})
	}
	said := runProbeRounds(t, rounds)
	assertNeverCalledRepetitive(t, said)
	for round, text := range said {
		if strings.Contains(text, "runway is spent") {
			t.Errorf("round %d ended the turn while paging one file", round)
		}
	}
}

// The control, and the property the fixed ladders could not express: a turn
// that keeps checking its work refills the account faster than it spends it, so
// it runs untouched however long it goes.
func TestVerifyingWorkKeepsTheTurnSolventIndefinitely(t *testing.T) {
	var rounds []roundCall
	for i := 1; i <= 3*evidence.RunwayStart; i++ {
		if i%3 == 0 {
			rounds = append(rounds, roundCall{"bash", fmt.Sprintf(`{"command":"go test ./internal/pkg%d"}`, i)})
			continue
		}
		rounds = append(rounds, roundCall{"read_file", fmt.Sprintf(`{"path":"internal/pkg%d/file.go"}`, i)})
	}
	for round, text := range runProbeRounds(t, rounds) {
		if text != "" {
			t.Errorf("round %d spoke on a working turn: %q", round, text)
		}
	}
}
