package main

import (
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"

	"reasonix/internal/instanceid"
)

func TestSecondLaunchRestoresTheRunningWindow(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	lock := singleInstanceLock(&App{})
	if lock == nil {
		t.Fatal("no lock: a second launch would open its own window over the same sessions")
	}
	if !strings.HasPrefix(lock.UniqueId, instanceid.Prefix+".") {
		t.Fatalf("UniqueId = %q, want the %s prefix", lock.UniqueId, instanceid.Prefix)
	}
	if lock.OnSecondInstanceLaunch == nil {
		t.Fatal("nothing answers a second launch, so it would be swallowed instead of raising the window")
	}
	lock.OnSecondInstanceLaunch(options.SecondInstanceData{})
}

func TestDevBuildRunsBesideAnInstalledOne(t *testing.T) {
	t.Setenv("REASONIX_DEV", "1")
	if lock := singleInstanceLock(&App{}); lock != nil {
		t.Fatalf("dev build took the lock (%q); it would be sent to the installed window instead", lock.UniqueId)
	}
}
