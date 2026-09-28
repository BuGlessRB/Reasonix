package boot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

func TestRememberPermissionRuleDoesNotCreateProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", filepath.Join(home, "state"))
	workspace := t.TempDir()
	result := rememberPermissionRule(config.RootsForHome(home), workspace, "Edit(src/app.go)")
	if !result.Saved || result.Err != nil {
		t.Fatalf("remember result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(workspace, "reasonix.toml")); !os.IsNotExist(err) {
		t.Fatalf("workspace config created without a request: %v", err)
	}
	if !strings.HasPrefix(result.Path, home+string(filepath.Separator)) {
		t.Fatalf("remembered rule path %q is outside Reasonix home %q", result.Path, home)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(result.Path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("remembered rule mode = %v, %v; want 0600", info, err)
		}
	}
}

func TestRememberedPermissionRulesSurviveReloadWithoutCrossingProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", filepath.Join(home, "state"))
	roots := config.RootsForHome(home)
	parent := t.TempDir()
	first := filepath.Join(parent, "frontend")
	second := filepath.Join(parent, "backend")
	for _, root := range []string{first, second} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if result := rememberPermissionRule(roots, first, "Bash(go test:*)"); !result.Saved || result.Err != nil {
		t.Fatalf("first remember = %+v", result)
	}
	if result := rememberPermissionRule(roots, second, "Edit(src/app.go)"); !result.Saved || result.Err != nil {
		t.Fatalf("second remember = %+v", result)
	}
	firstRules, err := rememberedPermissionRules(roots, first)
	if err != nil || len(firstRules) != 1 || firstRules[0] != "Bash(go test:*)" {
		t.Fatalf("first rules after reload = %v, %v", firstRules, err)
	}
	secondRules, err := rememberedPermissionRules(roots, second)
	if err != nil || len(secondRules) != 1 || secondRules[0] != "Edit(src/app.go)" {
		t.Fatalf("second rules after reload = %v, %v", secondRules, err)
	}
	if rememberedPermissionPath(roots, first) == rememberedPermissionPath(roots, second) {
		t.Fatal("two project roots share one permission state file")
	}
}

func TestBuildReportsRememberedRulesOnlyForTheirWorkspace(t *testing.T) {
	home := robustTempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", filepath.Join(home, "state"))
	first := t.TempDir()
	second := t.TempDir()
	if result := rememberPermissionRule(config.RootsForHome(home), first, "Bash(go test:*)"); !result.Saved || result.Err != nil {
		t.Fatalf("remember result = %+v", result)
	}
	for _, tc := range []struct {
		root string
		want int
	}{{first, 1}, {second, 0}} {
		ctrl, err := Build(context.Background(), Options{Home: home, WorkspaceRoot: tc.root, SessionDir: filepath.Join(home, "sessions"), Sink: event.Discard})
		if err != nil {
			t.Fatalf("Build(%s): %v", tc.root, err)
		}
		rules := ctrl.PermissionRules()
		if len(rules.Remembered) != tc.want {
			t.Errorf("Build(%s) remembered = %v, want %d", tc.root, rules.Remembered, tc.want)
		}
		if tc.want > 0 && (rules.Effective == nil || len(rules.Effective.Allow) != tc.want) {
			t.Errorf("Build(%s) effective rules = %+v", tc.root, rules.Effective)
		}
		ctrl.Close()
		if _, err := os.Stat(filepath.Join(tc.root, "reasonix.toml")); !os.IsNotExist(err) {
			t.Errorf("Build(%s) created workspace config: %v", tc.root, err)
		}
	}
}

func TestRememberedPermissionPathFollowsSymlinkTarget(t *testing.T) {
	roots := config.RootsForHome(t.TempDir())
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "second")
	link := filepath.Join(parent, "current")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	firstPath := rememberedPermissionPath(roots, link)
	if firstPath != rememberedPermissionPath(roots, first) {
		t.Fatal("symlink and its first target use different permission stores")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if firstPath == rememberedPermissionPath(roots, link) {
		t.Fatal("retargeted symlink inherited the former checkout's remembered permissions")
	}
}

func TestRememberedPermissionLoadFailureIsVisibleAndFailClosed(t *testing.T) {
	home := robustTempDir(t)
	t.Setenv("REASONIX_HOME", home)
	root := t.TempDir()
	path := rememberedPermissionPath(config.RootsForHome(home), root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[permissions\nallow = [\"Bash(go test:*)\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var notices []event.Event
	ctrl, err := Build(context.Background(), Options{
		Home: home, WorkspaceRoot: root, SessionDir: filepath.Join(home, "sessions"),
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice {
				notices = append(notices, e)
			}
		}),
	})
	if err != nil {
		t.Fatalf("Build should continue without malformed remembered rules: %v", err)
	}
	defer ctrl.Close()
	rules := ctrl.PermissionRules()
	if len(rules.Remembered) != 0 || rules.RememberedPath != path || rules.RememberedError == "" {
		t.Fatalf("remembered rules after malformed file = %+v", rules)
	}
	found := false
	for _, notice := range notices {
		if notice.Code == event.NoticeCodeRememberedPermissionLoadFailed {
			found = notice.Level == event.LevelWarn && strings.Contains(notice.Detail, path)
		}
	}
	if !found {
		t.Fatalf("missing stable load-failure notice: %+v", notices)
	}
}

func TestGlobalRememberPathDoesNotDuplicateUserConfig(t *testing.T) {
	roots := config.RootsForHome(t.TempDir())
	if got := rememberedPermissionPath(roots, ""); got != "" {
		t.Fatalf("global remembered path = %q, want empty", got)
	}
}
