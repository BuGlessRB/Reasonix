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
	var got struct {
		Servers []mcpEntry `json:"servers"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /mcp = %q: %v", strings.TrimSpace(string(body)), err)
	}
	if len(got.Servers) != 0 {
		t.Fatalf("GET /mcp servers = %+v, want none", got.Servers)
	}
}

// The slash list only holds what the user can type. A management surface has to
// see the skills that only the model can reach, or it cannot switch them off.
func TestSkillsListsWhatSlashCannotShow(t *testing.T) {
	ctrl := control.New(control.Options{
		Skills: []skill.Skill{
			{Name: "audit", Description: "read-only sweep", Scope: skill.ScopeBuiltin, RunAs: skill.RunSubagent, ReadOnly: true},
			{Name: "hinted", Description: "manual only", Scope: skill.ScopeProject, Invocation: "manual"},
		},
	})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	var got struct {
		Implicit bool         `json:"implicit"`
		Skills   []skillEntry `json:"skills"`
	}
	resp, err := http.Get(srv.URL + "/skills")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	by := map[string]skillEntry{}
	for _, e := range got.Skills {
		by[e.Name] = e
	}
	if len(by) != 2 {
		t.Fatalf("skills = %d entries, want 2: %+v", len(by), got.Skills)
	}
	if !by["audit"].ReadOnly || !by["audit"].Subagent {
		t.Errorf("audit lost its capability face: %+v", by["audit"])
	}
	if !by["audit"].Enabled {
		t.Error("audit reads as disabled; nothing disabled it")
	}
	if !by["hinted"].Manual {
		t.Error("a manual skill must not read as model-discoverable")
	}
	if by["audit"].Manual {
		t.Error("an auto skill must not read as manual")
	}
}

// The switch has to survive the request that set it, or the UI is reporting a
// state the next reload will contradict. With no workspace open the decision
// lands on the global layer — a project row keyed by nothing would resolve for
// nobody, so the switch would silently do nothing.
func TestSkillEnabledPersists(t *testing.T) {
	ctrl := control.New(control.Options{
		Skills: []skill.Skill{{Name: "audit", Scope: skill.ScopeBuiltin}},
	})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/skills/enabled", "application/json",
		strings.NewReader(`{"name":"audit","enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /skills/enabled = %d (%s), want 200", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if ctrl.SkillEnabled("audit") {
		t.Fatal("the skill is still enabled after the switch said off")
	}
}

// The same switch in one project must not answer for another. This is the
// asymmetry the skill surface used to have against MCP: a bare name in the
// user config disabled every same-named skill in every project at once.
func TestSkillSwitchIsScopedToItsProject(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	here, there := t.TempDir(), t.TempDir()
	skills := []skill.Skill{{Name: "deploy", Scope: skill.ScopeProject}}

	mine := control.New(control.Options{Skills: skills, WorkspaceRoot: here})
	defer mine.Close()
	theirs := control.New(control.Options{Skills: skills, WorkspaceRoot: there})
	defer theirs.Close()

	srv := httptest.NewServer(New(mine, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/skills/enabled", "application/json",
		strings.NewReader(`{"name":"deploy","enabled":false,"scope":"project"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if mine.SkillEnabled("deploy") {
		t.Fatal("the skill stayed on in the project that switched it off")
	}
	if !theirs.SkillEnabled("deploy") {
		t.Fatal("switching a skill off in one project also switched it off in another")
	}
}

// An unknown name is a client mistake, not a server failure: a 5xx would send
// the frontend into a retry loop over a name that will never resolve.
func TestMcpAdminRejectsUnknownServer(t *testing.T) {
	ctrl := control.New(control.Options{})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{"/mcp/enabled", `{"name":"nope","enabled":true}`, http.StatusBadRequest},
		{"/mcp/enabled", `{"enabled":true}`, http.StatusBadRequest},
		{"/mcp/reconnect", `{"name":"nope"}`, http.StatusBadGateway},
	} {
		resp, err := http.Post(srv.URL+tc.path, "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("POST %s %s = %d, want %d", tc.path, tc.body, resp.StatusCode, tc.want)
		}
	}
}

// Parsing is a separate step from installing so the user can be shown what a
// stranger's config would run before agreeing to it. It must never connect or
// persist anything on its own.
func TestMcpParsePreviewsWithoutInstalling(t *testing.T) {
	ctrl := control.New(control.Options{})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/mcp/parse", "application/json",
		strings.NewReader(`{"input":"{\"mcpServers\":{\"gh\":{\"command\":\"npx\",\"args\":[\"-y\",\"x\"],\"env\":{\"GITHUB_TOKEN\":\"ghp_literal\"}}}}"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /mcp/parse = %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var got struct {
		Servers []draftServer `json:"servers"`
		Risks   []draftRisk   `json:"risks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 1 || got.Servers[0].Name != "gh" {
		t.Fatalf("parse returned %+v, want one server named gh", got.Servers)
	}
	if got.Servers[0].Transport != "stdio" {
		t.Errorf("transport = %q, want stdio spelled out", got.Servers[0].Transport)
	}
	var secret bool
	for _, k := range got.Risks {
		if k.Kind == "secret" {
			secret = true
		}
	}
	if !secret {
		t.Error("a literal token reached the confirmation card unflagged")
	}
	// Nothing may exist yet: the user has not agreed to anything.
	if names := ctrl.ConfiguredMCPNames(); len(names) != 0 {
		t.Errorf("parse persisted %v", names)
	}
}

// A name collision is a client-side mistake with a precise remedy, so it comes
// back as a structured result rather than a 500 the UI can only print.
func TestMcpInstallRejectsDuplicateName(t *testing.T) {
	ctrl := control.New(control.Options{})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()

	// A command that cannot connect still exercises the pre-flight checks, and
	// leaves nothing behind — which is the contract being asserted.
	post := func(body string) map[string]any {
		resp, err := http.Post(srv.URL+"/mcp/install", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := post(`{"server":{"name":"","command":"true"},"scope":"user"}`)
	if got["state"] != "issue" {
		t.Errorf("a nameless server installed as %v, want issue", got["state"])
	}
	if names := ctrl.ConfiguredMCPNames(); len(names) != 0 {
		t.Errorf("a failed install left %v behind", names)
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
