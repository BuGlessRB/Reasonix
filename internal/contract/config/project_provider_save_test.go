package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestProjectEditDoesNotPersistBuiltInProviders(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(testenv.TempDir(t), "reasonix.toml")
	if err := os.WriteFile(path, []byte("[agent]\ncompact_ratio = 0.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EditConfigFile(path, func(c *Config) error { return c.SetCompactRatio(0.6) }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "[[providers]]") {
		t.Fatalf("unrelated project edit persisted built-in provider: %s", data)
	}
}

func TestNewProjectConfigDoesNotPersistBuiltInProviders(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(testenv.TempDir(t), "reasonix.toml")
	if err := EditConfigFile(path, func(c *Config) error { return c.SetCompactRatio(0.6) }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "[[providers]]") {
		t.Fatalf("new project config persisted built-in provider: %s", data)
	}
}

func TestProjectEditPreservesExplicitProvider(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(testenv.TempDir(t), "reasonix.toml")
	const provider = "[[providers]]\nname = \"custom\"\nkind = \"openai\"\nbase_url = \"https://example.com\"\nmodel = \"model-1\"\napi_key_env = \"CUSTOM_KEY\"\n"
	if err := os.WriteFile(path, []byte(provider), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EditConfigFile(path, func(c *Config) error { return c.SetCompactRatio(0.6) }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "[[providers]]") != 1 || !strings.Contains(string(data), `name        = "custom"`) || !strings.Contains(string(data), `base_url    = "https://example.com"`) {
		t.Fatalf("explicit project provider changed: %s", data)
	}
}
