package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"

	"reasonix/internal/store"
)

// The trajectory pane is built from wire frames, so the frames are what gets
// kept: replaying them reproduces the pane row for row, with no second schema
// to map through. internal/trajectory records a richer audit stream for offline
// analysis and answers a different question.
const wireLogMaxBytes = 8 << 20

// Only the kinds the pane draws a row for. Streamed text and reasoning arrive
// one frame per chunk and would be most of the file while contributing nothing
// to read back; keeping them out is what lets each write close its handle.
var wireLogKinds = map[string]bool{
	"turn_started": true, "turn_done": true, "message": true,
	"tool_dispatch": true, "tool_result": true, "usage": true,
	"guardian_assessment": true, "approval_request": true, "ask_request": true,
	"compaction_started": true, "compaction_done": true, "retrying": true,
	"steer": true, "context_maintenance": true, "completion_summary": true,
	"notice": true,
}

type wireLog struct {
	mu sync.Mutex
}

// attachWireLog mirrors qualifying broadcast frames to the current session's
// log. Subscribing is how the SSE handler reads the same stream, so the log
// sees exactly what a connected client would have.
func (s *Server) attachWireLog() {
	ch, unsubscribe := s.bc.Subscribe()
	go func() {
		defer unsubscribe()
		for frame := range ch {
			// Path is read per frame, not cached: /resume and /new swap it
			// underneath and the next row belongs to the new session.
			s.wire.write(store.SessionWireLog(s.ctl().SessionPath()), frame)
		}
	}()
}

func (w *wireLog) write(path string, frame []byte) {
	if path == "" {
		return
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(frame, &head); err != nil || !wireLogKinds[head.Kind] {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	// Reopened per write rather than held: a retained handle blocks the session
	// file's directory from being removed on Windows, and the filtered volume
	// makes the cost irrelevant.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	// A runaway turn must not fill the disk; drop rather than truncate, so what
	// is on disk stays a prefix of what happened.
	if st, err := f.Stat(); err == nil && st.Size() > wireLogMaxBytes {
		return
	}
	_, _ = f.Write(append(frame, '\n'))
}

func (s *Server) trajectory(w http.ResponseWriter, _ *http.Request) {
	path := store.SessionWireLog(s.ctl().SessionPath())
	if path == "" {
		writeJSON(w, []any{})
		return
	}
	s.wire.mu.Lock()
	data, err := os.ReadFile(path)
	s.wire.mu.Unlock()
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	out := []json.RawMessage{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, json.RawMessage(line))
	}
	writeJSON(w, out)
}
