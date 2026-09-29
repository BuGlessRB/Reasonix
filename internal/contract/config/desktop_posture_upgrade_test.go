package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUserConfigFile(t *testing.T, body string) string {
	t.Helper()
	isolateUserConfigHome(t)
	path := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The shipped "auto" was written into every saved config, so a file holding
// it goes back to the derived default, whatever its version; the values nobody
// shipped stay, including ones this build does not know.
func TestDesktopPostureUpgradeReleasesOnlyTheShippedAuto(t *testing.T) {
	for _, version := range []string{"6", "12"} {
		for _, tc := range []struct{ held, want string }{
			{"auto", ""},
			{"ask", "ask"},
			{"yolo", "yolo"},
			{"danger-full-access", "danger-full-access"},
		} {
			t.Run(version+"/"+tc.held, func(t *testing.T) {
				path := writeUserConfigFile(t, "config_version = "+version+"\n[desktop]\ndefault_tool_approval_mode = \""+tc.held+"\"\n")
				if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
					t.Fatal(err)
				}
				cfg := LoadForEdit(path)
				if got := cfg.Desktop.DefaultToolApprovalMode; got != tc.want {
					t.Fatalf("after upgrade the file holds %q, want %q", got, tc.want)
				}
				if got := cfg.ConfigVersion; strings.TrimSpace(version) == "12" && got != 12 {
					t.Fatalf("releasing the posture moved config_version to %d", got)
				}
			})
		}
	}
}

// The release runs once. An "auto" chosen after it is the person's and stays,
// and the release never moves config_version: 7 through 12 are the 1.x line's
// own upgrades, and a file it finds marked past them is one it skips them on.
func TestDesktopPostureUpgradeRunsOnce(t *testing.T) {
	path := writeUserConfigFile(t, "config_version = 6\n[desktop]\ndefault_tool_approval_mode = \"auto\"\n")
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	if cfg.ConfigVersion != 6 {
		t.Fatalf("config_version = %d after the release, want 6", cfg.ConfigVersion)
	}
	if err := cfg.SetDesktopDefaultToolApprovalMode("auto"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if changed, err := ApplyUserConfigUpgradesOnStartup(path); err != nil || changed {
		t.Fatalf("second start = %v, %v; want untouched", changed, err)
	}
	if got := LoadForEdit(path).DesktopDefaultToolApprovalMode(); got != "auto" {
		t.Fatalf("a chosen auto became %q", got)
	}
}

// A first launch with no config records the release, so an "auto" the person
// picks before the next launch is not taken for the shipped one.
func TestDesktopPostureUpgradeMarksAFreshInstall(t *testing.T) {
	isolateUserConfigHome(t)
	path := UserConfigPath()
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	if err := cfg.SetDesktopDefaultToolApprovalMode("auto"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadForEdit(path).DesktopDefaultToolApprovalMode(); got != "auto" {
		t.Fatalf("an auto chosen on a fresh install became %q", got)
	}
}

// Releasing the posture edits that one key in place: unknown keys, comments
// and every other line keep their bytes.
func TestDesktopPostureUpgradeEditsInPlace(t *testing.T) {
	body := "config_version = 12\n# mine\n[desktop]\nlayout_style = \"classic\"   # 1.x\ndefault_tool_approval_mode = \"auto\"\ntheme = \"dark\"\n"
	path := writeUserConfigFile(t, body)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(body, "default_tool_approval_mode = \"auto\"\n", "", 1)
	if string(raw) != want {
		t.Fatalf("release rewrote more than the key:\n%s\nwant:\n%s", raw, want)
	}
}

// An unset posture renders as a comment, so saving a config never turns the
// default into a choice.
func TestDesktopPostureUnsetSurvivesASave(t *testing.T) {
	isolateUserConfigHome(t)
	path := UserConfigPath()
	if err := Default().SaveTo(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "default_tool_approval_mode") {
			t.Fatalf("a default config wrote %q", line)
		}
	}
	cfg := LoadForEdit(path)
	if got := cfg.DesktopDefaultToolApprovalMode(); got != "" {
		t.Fatalf("a saved default config reads back %q, want unset", got)
	}
	cfg.UI.Theme = "dark"
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadForEdit(path).DesktopDefaultToolApprovalMode(); got != "" {
		t.Fatalf("an unrelated in-place save wrote the posture %q", got)
	}
}
