package plugin

import (
	"context"
	"strings"
	"testing"
)

// A lazily-started server whose context ends says which of the two things
// happened. Before this, both arrived as "context canceled" in 0-4ms with no
// stderr, which reads like the server itself failed to launch.
func TestLaunchFailureNamesWhoCancelled(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  string
	}{
		{"host closed", ErrHostClosed, "MCP host shut down"},
		{"server removed", ErrServerRemoved, "removed, disabled, or reconnected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(tc.cause)
			_, err := start(ctx, context.Background(), // A command that exists: resolving the executable happens before the
				// context is consulted, so a missing one would fail earlier and for
				// another reason.
				Spec{Name: "probe", Command: "/bin/echo"})
			if err == nil {
				t.Fatal("a cancelled context started a server")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to name %q", err.Error(), tc.want)
			}
		})
	}
}
