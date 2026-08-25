package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestSecondLaunchRestoresTheRunningWindow(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	lock := singleInstanceLock(&App{})
	if lock == nil {
		t.Fatal("no lock: a second launch would open its own window over the same sessions")
	}
	if !strings.HasPrefix(lock.UniqueId, singleInstanceIDPrefix+".") {
		t.Fatalf("UniqueId = %q, want the %s prefix", lock.UniqueId, singleInstanceIDPrefix)
	}
	if lock.OnSecondInstanceLaunch == nil {
		t.Fatal("nothing answers a second launch, so it would be swallowed instead of raising the window")
	}
	lock.OnSecondInstanceLaunch(options.SecondInstanceData{})
}

func TestInstanceIDScopesToTheDataHome(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", first)
	want := singleInstanceID()
	t.Setenv("REASONIX_HOME", filepath.Join(first, "."))
	if got := singleInstanceID(); got != want {
		t.Fatalf("one data home spelled two ways produced %q and %q", got, want)
	}
	t.Setenv("REASONIX_HOME", second)
	if got := singleInstanceID(); got == want {
		t.Fatalf("two data homes share id %q, so the second launch would be refused a window", got)
	}
}

// A home that has not been created yet is the first launch, and it must hash to
// the same place the second one will find.
func TestInstanceIDResolvesAMissingHomeThroughASymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	t.Setenv("REASONIX_HOME", filepath.Join(real, "not-created", "home"))
	want := singleInstanceID()
	t.Setenv("REASONIX_HOME", filepath.Join(alias, "not-created", "home"))
	if got := singleInstanceID(); got != want {
		t.Fatalf("the same home reached through a symlink produced %q and %q", got, want)
	}
}

func TestDevBuildRunsBesideAnInstalledOne(t *testing.T) {
	t.Setenv("REASONIX_DEV", "1")
	if lock := singleInstanceLock(&App{}); lock != nil {
		t.Fatalf("dev build took the lock (%q); it would be sent to the installed window instead", lock.UniqueId)
	}
}
