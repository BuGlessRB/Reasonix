package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Rejecting an operation without naming the valid ones leaves the caller to
// guess or read source. taskMonitorCommand in task.go already lists its
// subcommands on the same kind of failure; these three did not.
func TestUnknownOperationNamesTheValidOnes(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(args []string, out *bytes.Buffer) int
		want []string
	}{
		{
			name: "hook",
			run:  func(a []string, o *bytes.Buffer) int { return runHookCommand(a, o) },
			want: []string{"list", "status"},
		},
		{
			name: "session",
			run:  func(a []string, o *bytes.Buffer) int { return runSessionCommand(a, o) },
			want: []string{"list", "show", "status", "recovery"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if rc := tc.run([]string{"bogus", "--json"}, &out); rc == 0 {
				t.Fatal("unknown operation returned success")
			}
			var payload struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("machine output is not JSON: %v (%s)", err, out.String())
			}
			if payload.Error.Code != "unknown_command" {
				t.Fatalf("code = %q, want unknown_command", payload.Error.Code)
			}
			if !strings.Contains(payload.Error.Message, "bogus") {
				t.Fatalf("message does not echo the rejected operation: %s", payload.Error.Message)
			}
			for _, op := range tc.want {
				if !strings.Contains(payload.Error.Message, op) {
					t.Fatalf("message omits valid operation %q: %s", op, payload.Error.Message)
				}
			}
		})
	}
}
