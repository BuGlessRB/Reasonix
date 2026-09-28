package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// The hidden location is used only when the user-global flag is on: the plain
// file wins when it exists, and the hidden file is otherwise the target.
func TestProjectConfigPathHonoursHidden(t *testing.T) {
	root := testenv.TempDir(t)

	if got := projectConfigPathForRoot(root, false); got != filepath.Join(root, "reasonix.toml") {
		t.Fatalf("hidden off = %q, want reasonix.toml", got)
	}
	if got := projectConfigPathForRoot(root, true); got != filepath.Join(root, ".reasonix", "config.toml") {
		t.Fatalf("hidden on, none present = %q, want .reasonix/config.toml", got)
	}

	plain := filepath.Join(root, "reasonix.toml")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := projectConfigPathForRoot(root, true); got != plain {
		t.Fatalf("hidden on, plain present = %q, want the plain file to win", got)
	}
}

func TestLoadReadsHiddenProjectConfigWhenEnabled(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "project_config_hidden = true\n")

	project := testenv.TempDir(t)
	if err := os.MkdirAll(filepath.Join(project, ".reasonix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectDefaultTestConfig(t, filepath.Join(project, ".reasonix"), "config.toml", "default_model = \"hidden/model\"\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultModel != "hidden/model" {
		t.Fatalf("DefaultModel = %q, want the hidden project config to be read", cfg.DefaultModel)
	}
}

func TestLoadIgnoresHiddenProjectConfigWhenDisabled(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "default_model = \"user/model\"\n")

	project := testenv.TempDir(t)
	if err := os.MkdirAll(filepath.Join(project, ".reasonix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectDefaultTestConfig(t, filepath.Join(project, ".reasonix"), "config.toml", "default_model = \"hidden/model\"\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultModel != "user/model" {
		t.Fatalf("DefaultModel = %q, want the hidden project config ignored", cfg.DefaultModel)
	}
}

func TestHiddenProjectConfigPlainFileWins(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "project_config_hidden = true\n")

	project := testenv.TempDir(t)
	if err := os.MkdirAll(filepath.Join(project, ".reasonix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectDefaultTestConfig(t, filepath.Join(project, ".reasonix"), "config.toml", "default_model = \"hidden/model\"\n")
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "default_model = \"plain/model\"\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultModel != "plain/model" {
		t.Fatalf("DefaultModel = %q, want reasonix.toml to win", cfg.DefaultModel)
	}
}

// The setting is user-global: a repository cannot turn it on for itself, and the
// load says so rather than leaving the user reading a config their reasonix.toml
// never reached.
func TestProjectCannotEnableHiddenProjectConfig(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "")

	project := testenv.TempDir(t)
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "project_config_hidden = true\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectConfigHidden {
		t.Fatal("project reasonix.toml turned on the hidden project config")
	}
	if !strings.Contains(strings.Join(cfg.LoadWarnings(), "\n"), "project_config_hidden") {
		t.Fatalf("no warning that the project project_config_hidden was ignored: %v", cfg.LoadWarnings())
	}
}

// The user scope renders the toggle so a full-file config rewrite keeps it; the
// project scope never does, because a repository cannot set it.
func TestRenderPersistsProjectConfigHidden(t *testing.T) {
	cfg := Default()
	cfg.ProjectConfigHidden = true

	user := RenderTOMLForScope(cfg, RenderScopeUser)
	if !strings.Contains(user, "project_config_hidden = true") {
		t.Fatalf("user scope dropped project_config_hidden:\n%s", user)
	}
	if project := RenderTOMLForScope(cfg, RenderScopeProject); strings.Contains(project, "project_config_hidden") {
		t.Fatalf("project scope rendered project_config_hidden:\n%s", project)
	}
}
