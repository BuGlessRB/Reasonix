package migration

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/testenv"
)

func seed(t *testing.T, dir, name, file string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, file), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// defaultRoots puts the OS defaults in a temp tree and clears both redirects,
// so what runs is the path a real install takes.
func defaultRoots(t *testing.T) string {
	t.Helper()
	base := testenv.TempDir(t)
	t.Setenv("HOME", base)
	t.Setenv("USERPROFILE", base)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, ".config"))
	t.Setenv("AppData", filepath.Join(base, "AppData"))
	t.Setenv("REASONIX_HOME", "")
	t.Setenv("REASONIX_STATE_HOME", "")
	return base
}

// A run that redirects its roots by environment is a scratch run; it must not
// pull the production install across, which is the rule the legacy importers
// already follow.
func TestAdoptSkipsWhenTheRootsAreRedirectedForThisRun(t *testing.T) {
	base := testenv.TempDir(t)
	home := filepath.Join(base, "home")
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", filepath.Join(base, "state"))
	seed(t, home, "appearance", "wp.png")
	if got := AdoptRelocatedStateEntries(event.Discard); len(got) != 0 {
		t.Fatalf("a redirected run adopted %v", got)
	}
}

// The one this exists for: appearance, themes and repair were written under the
// state root before it listed them, so a relocation moved the rest and left
// these where they were. Relocation is a config table, not an environment
// variable — the environment path is the isolated run the guard above refuses.
func TestAdoptBringsAcrossWhatTheOldMoveLeft(t *testing.T) {
	base := defaultRoots(t)
	home := config.ReasonixHomeDir()
	if home == "" {
		t.Skip("no default home on this platform")
	}
	state := filepath.Join(base, "moved-state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[storage]\nstate = " + strconv.Quote(state) + "\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InvalidateStorageDirs()
	if got := config.MemoryUserDir(); got != state {
		t.Fatalf("relocation did not take: state root is %q, want %q", got, state)
	}

	seed(t, home, "appearance", "wp.png")
	seed(t, home, "themes", "pack.json")
	seed(t, home, "repair", "backup.toml")
	seed(t, home, "sessions", "old.jsonl")

	adopted := AdoptRelocatedStateEntries(event.Discard)
	if len(adopted) != 3 {
		t.Fatalf("adopted %v, want appearance, themes and repair", adopted)
	}
	for _, name := range []string{"appearance", "themes", "repair"} {
		if !isDir(filepath.Join(state, name)) {
			t.Errorf("%s was left behind", name)
		}
	}
	// sessions was always owned, so the move already took it; bringing it
	// across would resurrect history the user has deleted since.
	if isDir(filepath.Join(state, "sessions")) {
		t.Error("sessions was adopted; the move already owned it")
	}
}

// Never over an entry the new root already has: that directory is this
// install's, and copying into it would bring back what was deleted.
func TestAdoptLeavesAnEntryTheNewRootAlreadyHas(t *testing.T) {
	base := defaultRoots(t)
	home := config.ReasonixHomeDir()
	if home == "" {
		t.Skip("no default home on this platform")
	}
	state := filepath.Join(base, "moved-state")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[storage]\nstate = " + strconv.Quote(state) + "\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InvalidateStorageDirs()
	seed(t, home, "themes", "old.json")
	seed(t, state, "themes", "current.json")
	AdoptRelocatedStateEntries(event.Discard)
	if _, err := os.Stat(filepath.Join(state, "themes", "old.json")); err == nil {
		t.Fatal("an entry the new root already had was written into")
	}
}
