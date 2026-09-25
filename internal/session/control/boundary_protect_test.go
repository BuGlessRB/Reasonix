package control

import (
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
)

// The settings switch is the config key: off persists, reads back, and survives
// a later save of the other sandbox fields.
func TestSandboxSettingsPersistChangedFileProtection(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	c := &Controller{controllerDeps: controllerDeps{workspaceRoot: testenv.TempDir(t)}}

	s := c.SandboxSettings()
	if !s.ProtectChangedFiles {
		t.Fatal("changed-file protection should default on")
	}
	s.ProtectChangedFiles = false
	if err := c.SaveSandboxSettings(s); err != nil {
		t.Fatal(err)
	}
	if c.SandboxSettings().ProtectChangedFiles {
		t.Fatal("turning it off did not read back")
	}
	if config.LoadForEdit(config.UserConfigPath()).Tools.ChangedFilesProtected() {
		t.Fatal("turning it off did not reach the config file")
	}

	s = c.SandboxSettings()
	s.AllowWrite = []string{"/tmp/scratch"}
	if err := c.SaveSandboxSettings(s); err != nil {
		t.Fatal(err)
	}
	if c.SandboxSettings().ProtectChangedFiles {
		t.Fatal("saving another sandbox field turned protection back on")
	}
}
