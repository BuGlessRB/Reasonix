package boot

import (
	"context"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
)

// TestGoalOnlyToolsLeaveTheSchemaWhenUnreachable is an effect test at the
// provider boundary: a headless `run` can never arm a Goal turn, so shipping
// update_goal only bought a definition the model calls and the host rejects.
// A whole-session absence shortens the cache-stable prefix without churning it.
func TestGoalOnlyToolsLeaveTheSchemaWhenUnreachable(t *testing.T) {
	for _, tc := range []struct {
		name        string
		unreachable bool
		wantGoal    bool
	}{
		{name: "reachable keeps update_goal", unreachable: false, wantGoal: true},
		{name: "unreachable drops update_goal", unreachable: true, wantGoal: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfigHome(t)
			dir := robustTempDir(t)
			t.Chdir(dir)

			registerBootTokenProfileTestProvider()
			prov := testutil.NewMock("goal-surface", testutil.Turn{Text: "done"})
			setBootTokenProfileTestProvider(t, prov)
			writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[[providers]]
name = "test-model"
kind = "boot-token-profile-test"
model = "x"
`)

			ctrl, err := Build(context.Background(), Options{
				Sink:                 event.Discard,
				GoalTurnsUnreachable: tc.unreachable,
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			defer ctrl.Close()
			if err := ctrl.Run(context.Background(), "go"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			reqs := prov.Requests()
			if len(reqs) != 1 {
				t.Fatalf("requests = %d, want 1", len(reqs))
			}
			if got := requestHasTool(reqs[0], "update_goal"); got != tc.wantGoal {
				t.Fatalf("update_goal present = %v, want %v; tools=%v",
					got, tc.wantGoal, toolSchemaNames(reqs[0].Tools))
			}
			// The execution surface is untouched either way.
			for _, want := range []string{"bash", "read_file", "edit_file", "write_file"} {
				if !requestHasTool(reqs[0], want) {
					t.Fatalf("missing execution tool %q; tools=%v", want, toolSchemaNames(reqs[0].Tools))
				}
			}
		})
	}
}
