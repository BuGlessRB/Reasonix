package control

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
)

func TestPermissionRulesReportsRememberedProjectGrants(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	workspace := testenv.TempDir(t)
	other := testenv.TempDir(t)
	store := config.NewProjectGrantStore(config.Roots{}.Home())
	if err := store.Update(workspace, func(g config.ProjectGrant) (config.ProjectGrant, error) {
		g.Allow = []string{"Bash(go test:*)"}
		return g, nil
	}); err != nil {
		t.Fatal(err)
	}

	ctrl := New(Options{WorkspaceRoot: workspace})
	defer ctrl.Close()
	rules := ctrl.PermissionRules()
	if rules.RememberedPath != store.Path() || rules.RememberedError != "" || len(rules.Remembered) != 1 || rules.Remembered[0] != "Bash(go test:*)" {
		t.Fatalf("project rules = %+v", rules)
	}

	otherCtrl := New(Options{WorkspaceRoot: other})
	defer otherCtrl.Close()
	if got := otherCtrl.PermissionRules(); len(got.Remembered) != 0 {
		t.Fatalf("other workspace inherited grants: %+v", got)
	}

	if err := os.WriteFile(store.Path(), []byte("broken JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ctrl.PermissionRules(); got.RememberedError == "" || len(got.Remembered) != 0 || got.RememberedPath != filepath.Join(home, "project-grants.json") {
		t.Fatalf("unreadable grant store was hidden or applied: %+v", got)
	}
}
