package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/command"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/skill"
)

func fetchSlash(t *testing.T, ctrl *control.Controller) []slashEntry {
	t.Helper()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/slash")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /slash = %d, want 200", resp.StatusCode)
	}
	var got []slashEntry
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

// Submit resolves a custom command before a skill of the same name, so a menu
// built from this endpoint must offer the entry that would actually run.
func TestSlashCommandShadowsSameNamedSkill(t *testing.T) {
	ctrl := control.New(control.Options{
		Commands: []command.Command{{Name: "review", Description: "the command"}},
		Skills: []skill.Skill{
			{Name: "review", Description: "the skill", Scope: skill.ScopeProject},
			{Name: "audit", Description: "read-only sweep", Scope: skill.ScopeBuiltin, RunAs: skill.RunSubagent},
		},
	})
	defer ctrl.Close()

	got := fetchSlash(t, ctrl)
	byName := map[string][]slashEntry{}
	for _, e := range got {
		byName[e.Name] = append(byName[e.Name], e)
	}
	if n := len(byName["review"]); n != 1 {
		t.Fatalf("review appears %d times, want 1", n)
	}
	if k := byName["review"][0].Kind; k != "command" {
		t.Errorf("review kind = %q, want command", k)
	}

	audit := byName["audit"]
	if len(audit) != 1 {
		t.Fatalf("audit appears %d times, want 1", len(audit))
	}
	if !audit[0].Subagent {
		t.Error("audit lost its subagent flag; the menu cannot say it needs a task")
	}
	if audit[0].Scope != string(skill.ScopeBuiltin) {
		t.Errorf("audit scope = %q, want builtin", audit[0].Scope)
	}
}

// A session with no plugin host still has to answer with a list. JSON null
// would reach the frontend as a value it cannot iterate.
func TestMcpWithoutHostReturnsEmptyList(t *testing.T) {
	ctrl := control.New(control.Options{})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(body)); got != "[]" {
		t.Fatalf("GET /mcp = %q, want []", got)
	}
}

// A hidden command is invocable but deliberately absent from listings.
func TestSlashOmitsHiddenCommands(t *testing.T) {
	ctrl := control.New(control.Options{
		Commands: []command.Command{
			{Name: "ship", Description: "visible"},
			{Name: "sh", Description: "compat alias", Hidden: true},
		},
	})
	defer ctrl.Close()

	for _, e := range fetchSlash(t, ctrl) {
		if e.Name == "sh" {
			t.Fatal("hidden command reached the slash listing")
		}
	}
}
