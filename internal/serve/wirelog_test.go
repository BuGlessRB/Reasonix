package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Reconnecting rebuilds the trajectory pane from this log, so every frame the
// pane draws a row for has to survive it. Rounds are where a turn's clock
// actually goes; dropping their frames left a replayed pane with tool marks and
// no trunk to hang them on, and no anchor for the usage that names them.
func TestWireLogKeepsTheFramesTheTimelineDrawsRowsFor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.wire.jsonl")
	var w wireLog
	for _, frame := range []string{
		`{"kind":"turn_started"}`,
		`{"kind":"stream_attempt","streamAttempt":{"id":"sa-1-7","action":"begin"}}`,
		`{"kind":"reasoning","text":"one chunk of many"}`,
		`{"kind":"text","text":"one chunk of many"}`,
		`{"kind":"tool_dispatch","tool":{"id":"t1","name":"bash"}}`,
		`{"kind":"tool_result","tool":{"id":"t1","name":"bash","durationMs":420}}`,
		`{"kind":"stream_attempt","streamAttempt":{"id":"sa-1-7","action":"commit"}}`,
		`{"kind":"usage","usage":{"attemptId":"sa-1-7","totalTokens":7}}`,
		`{"kind":"turn_done","cancelled":true}`,
	} {
		w.write(path, []byte(frame))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read wire log: %v", err)
	}
	var kept []string
	for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		var head struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &head); err != nil {
			t.Fatalf("wire log line is not a frame: %q", line)
		}
		kept = append(kept, head.Kind)
	}

	// Streamed text and reasoning arrive one frame per chunk and draw no row of
	// their own; everything else here is a row a reader expects to find.
	want := []string{
		"turn_started", "stream_attempt", "tool_dispatch", "tool_result",
		"stream_attempt", "usage", "turn_done",
	}
	if strings.Join(kept, ",") != strings.Join(want, ",") {
		t.Fatalf("wire log kept %v, want %v", kept, want)
	}
}
