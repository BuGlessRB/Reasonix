package delegation

import "testing"

func TestOnlyAForegroundWriterObservesTheWorkspace(t *testing.T) {
	task := &TaskTool{workspaceRoot: "/ws"}
	if got := task.observeRootFor(false); got != "/ws" {
		t.Fatalf("foreground writer observe root = %q, want the workspace", got)
	}
	if got := task.observeRootFor(true); got != "" {
		t.Fatalf("background writer observe root = %q, want none: its walk would claim its siblings' writes", got)
	}
}
