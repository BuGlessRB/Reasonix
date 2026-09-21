package main

import (
	"context"
	"testing"

	"reasonix/internal/skill/skillwatch"
)

// The host owns one watcher for its whole lifetime: a second service would be a
// second helper process, and a replacement would strand the subscriptions the
// first one holds. An existing service must be reused, never re-created.
func TestSharedSkillWatchServiceIsReusedPerApp(t *testing.T) {
	app := NewApp()
	seeded := &skillwatch.Service{}
	app.skillWatch = seeded
	for range 3 {
		if got := app.sharedSkillWatchService(); got != seeded {
			t.Fatal("an existing host skill watch service must be reused, not replaced")
		}
	}
}

// The desktop must apply the same production-binary gate as boot. A real
// service created here would spawn a helper through os.Executable(), and in a
// package test binary that helper is this test binary — it would re-run the
// suite in a child process.
func TestSharedSkillWatchServiceIsNilInTestBinaries(t *testing.T) {
	if service := NewApp().sharedSkillWatchService(); service != nil {
		_ = service.Close()
		t.Fatal("a package test binary must not create a skill watch service")
	}
}

// The host watcher must be closed by shutdown: boot.Build deliberately leaves a
// caller-owned service alone, so nothing else closes it. A real helper cannot
// be observed from here (creating a service in a test binary would spawn this
// binary), so this pins the step wiring instead.
func TestShutdownRunsTheSkillWatchStep(t *testing.T) {
	app := NewApp()
	app.shutdown(context.Background())
	state := app.shutdownState()
	state.mu.Lock()
	ran := state.finished["skill-watch-service"]
	state.mu.Unlock()
	if !ran {
		t.Fatal("shutdown did not run the skill-watch step")
	}
}
