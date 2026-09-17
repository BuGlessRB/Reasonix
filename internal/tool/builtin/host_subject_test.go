package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/permission"
)

// A field the tool does not declare cannot become what the gate reads: the
// subject is the one the host projected, whatever else the model sent.
func TestPermissionSubjectIsTheProjectedOneWhateverTheModelAdds(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		got  json.RawMessage
		want string
	}{
		{"browser_open", browserOpen{}.PermissionArgs(ctx, json.RawMessage(`{"url":"https://example.com/a","path":"/etc/passwd","command":"x"}`)), "https://example.com"},
		{"browser_read unbound", browserRead{}.PermissionArgs(ctx, json.RawMessage(`{"what":"snapshot","origin":"https://model.example","file_path":"x"}`)), ""},
		{"browser_act unbound", browserAct{}.PermissionArgs(ctx, json.RawMessage(`{"steps":[],"origin":"https://model.example","command":"x"}`)), ""},
	}
	for _, tc := range cases {
		if got := permission.Subject(tc.got); got != tc.want {
			t.Errorf("%s: subject = %q (from %s), want %q", tc.name, got, tc.got, tc.want)
		}
	}
	var kept map[string]any
	_ = json.Unmarshal(cases[2].got, &kept)
	if _, ok := kept["steps"]; !ok || len(kept) != 1 {
		t.Errorf("the gate should read the declared steps and nothing else: %s", cases[2].got)
	}
}
