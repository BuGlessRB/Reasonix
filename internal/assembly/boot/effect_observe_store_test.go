package boot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/contract/observe"
	"reasonix/internal/runtime/agent/testutil"
)

const storeMark = "SCHEDULE-STORE-BODY"

// The store sits inside the workspace here, so the read scope alone would let
// the run see it; the protected set is what keeps a run from reading its own
// limits and the results of the other schedules.
func TestEffectObserveCannotReadTheScheduleStore(t *testing.T) {
	root := observeProject(t)
	state := filepath.Join(root, "state")
	t.Setenv("REASONIX_STATE_HOME", state)
	store := filepath.Join(state, "schedules")
	writeFile(t, store, "schedules.json", `{"note":"`+storeMark+`"}`)
	writeFile(t, filepath.Join(store, "results"), "r.json", storeMark)
	writeFile(t, root, "inside.go", "package in\n// "+insideMark+"\n")
	slash := filepath.ToSlash(store)
	calls := [][3]string{
		{"read", "read_file", `{"path":"` + slash + `/schedules.json"}`},
		{"read-rel", "read_file", `{"path":"state/schedules/results/r.json"}`},
		{"ls", "ls", `{"path":"` + slash + `"}`},
		{"glob", "glob", `{"pattern":"state/schedules/**/*.json"}`},
		{"grep-dir", "grep", `{"pattern":"` + storeMark + `","path":"state/schedules"}`},
		{"grep-walk", "grep", `{"pattern":"` + storeMark + `","path":"."}`},
		{"idx", "code_index", `{"action":"outline","path":"state/schedules"}`},
		{"ok", "read_file", `{"path":"inside.go"}`},
	}
	var turns []testutil.Turn
	for _, c := range calls {
		turns = append(turns, call(c[0], c[1], c[2]))
	}
	prov := testutil.NewMock("observe", append(turns, testutil.Turn{Text: "done"})...)
	ctrl, _ := buildObserved(t, root, prov, observe.RunContext{ScheduleID: "s", TriggerID: "t"})
	if err := ctrl.Run(context.Background(), "read"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	results := toolResults(prov.Requests())
	for _, c := range calls[:len(calls)-1] {
		if strings.Contains(results[c[0]], storeMark) {
			t.Errorf("%s read the schedule store: %q", c[0], results[c[0]])
		}
	}
	if !strings.Contains(results["ok"], insideMark) {
		t.Fatalf("the control read of an ordinary file failed, so the refusals prove nothing: %q", results["ok"])
	}
}
