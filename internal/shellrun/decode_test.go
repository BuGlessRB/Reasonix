package shellrun

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const codePageLine = "FIND: 参数格式不正确\r\n"

func gbkBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A Windows console tool answers in the machine's code page. The bytes used to
// be kept as a Go string unchanged, and JSON then coerced every invalid one to
// U+FFFD: a run that failed four times told the model nothing about why.
func TestShellOutputInTheMachineCodePageIsReadable(t *testing.T) {
	if got := decodeShellOutput(gbkBytes(t, codePageLine)); got != codePageLine {
		t.Fatalf("decodeShellOutput = %q, want %q", got, codePageLine)
	}
}

// UTF-8 output is passed through byte for byte, including a tail the buffer cut
// mid-character — re-reading that as the code page would invent mojibake where
// truncation was the only problem.
func TestUTF8OutputIsNotReinterpreted(t *testing.T) {
	full := "参数格式不正确 ok\n"
	if got := decodeShellOutput([]byte(full)); got != full {
		t.Fatalf("valid UTF-8 was rewritten: %q", got)
	}
	if cut := []byte(full)[1:]; decodeShellOutput(cut) != string(cut) {
		t.Fatal("a front-truncated tail was reinterpreted")
	}
	if tail := []byte(full)[:len(full)-4]; decodeShellOutput(tail) != string(tail) {
		t.Fatal("a back-truncated tail was reinterpreted")
	}
	if got := decodeShellOutput(nil); got != "" {
		t.Fatalf("empty output = %q", got)
	}
	if got := decodeShellOutput([]byte("plain ascii")); !strings.Contains(got, "ascii") {
		t.Fatalf("ascii = %q", got)
	}
}

// The collector is what a run's output actually passes through, so the decode
// has to sit there rather than in a helper the caller could stop using.
func TestTheCollectorDecodesWhatTheChildWrote(t *testing.T) {
	gbk := gbkBytes(t, codePageLine)
	c := newOutputCollector(1<<20, 1<<10)
	if _, err := c.combined.Write(gbk); err != nil {
		t.Fatal(err)
	}
	if _, err := c.tail.Write(gbk); err != nil {
		t.Fatal(err)
	}
	if got := c.combinedString(); got != codePageLine {
		t.Fatalf("combined = %q, want %q", got, codePageLine)
	}
	if got := c.tailString(); got != codePageLine {
		t.Fatalf("tail = %q, want %q", got, codePageLine)
	}
}
