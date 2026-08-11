package evidence

import (
	"encoding/json"
	"fmt"
	"testing"
)

func readReceipt(path string) Receipt {
	return ReceiptFromToolCall("read_file", json.RawMessage(`{"path":"`+path+`"}`), true, true)
}

func bashReceipt(command string, success bool) Receipt {
	args, _ := json.Marshal(map[string]string{"command": command})
	return ReceiptFromToolCall("bash", args, success, false)
}

func withOutput(r Receipt) Receipt {
	r.OutputBytes = 64
	return r
}

// A path already read can still answer a question never asked. Keying read
// novelty on the path alone made the standard investigation moves — grepping
// one package for a second symbol, paging through a long file — score as
// repeats, and a turn doing exactly that was told it had produced no new
// evidence.
func TestReadNoveltyFollowsTheQuestionNotJustThePath(t *testing.T) {
	tr := NewOutcomeTracker()

	grep := func(pattern string) Receipt {
		args := json.RawMessage(fmt.Sprintf(`{"pattern":%q,"path":"internal/agent"}`, pattern))
		return withOutput(ReceiptFromToolCall("grep", args, true, true))
	}
	if s := tr.ScoreRound([]Receipt{grep("Compose")}); s.Exploration != 1 {
		t.Fatalf("first grep = %+v, want exploration 1", s)
	}
	if s := tr.ScoreRound([]Receipt{grep("Receipt")}); s.Exploration != 1 {
		t.Fatalf("new pattern over a read path = %+v, want exploration 1", s)
	}
	if s := tr.ScoreRound([]Receipt{grep("Receipt")}); s.Exploration != 0 {
		t.Fatalf("identical grep = %+v, want exploration 0", s)
	}

	window := func(offset int) Receipt {
		args := json.RawMessage(fmt.Sprintf(`{"path":"internal/agent/agent.go","offset":%d,"limit":300}`, offset))
		return withOutput(ReceiptFromToolCall("read_file", args, true, true))
	}
	if s := tr.ScoreRound([]Receipt{window(0)}); s.Exploration != 1 {
		t.Fatalf("first window = %+v, want exploration 1", s)
	}
	if s := tr.ScoreRound([]Receipt{window(300)}); s.Exploration != 1 {
		t.Fatalf("next window of the same file = %+v, want exploration 1", s)
	}
	if s := tr.ScoreRound([]Receipt{window(300)}); s.Exploration != 0 {
		t.Fatalf("identical window = %+v, want exploration 0", s)
	}

	// A first read of a path still counts once, not twice.
	if s := tr.ScoreRound([]Receipt{withOutput(readReceipt("a.go"))}); s.Exploration != 1 {
		t.Fatalf("new path = %+v, want exploration 1", s)
	}
	if s := tr.ScoreRound([]Receipt{withOutput(readReceipt("a.go"))}); s.Exploration != 0 {
		t.Fatalf("identical read = %+v, want exploration 0", s)
	}
}

func TestLedgerReceiptsSince(t *testing.T) {
	l := &Ledger{}
	l.Record(bashReceipt("one", true))
	l.Record(bashReceipt("two", true))
	if got := len(l.ReceiptsSince(1)); got != 1 {
		t.Fatalf("receipts since 1 = %d, want 1", got)
	}
	if got := l.ReceiptsSince(5); got != nil {
		t.Fatalf("out-of-range mark must yield nil, got %v", got)
	}
	var nilLedger *Ledger
	if got := nilLedger.ReceiptsSince(0); got != nil {
		t.Fatalf("nil ledger must yield nil")
	}
}
