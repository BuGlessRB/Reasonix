package boot

import (
	"io"
	"testing"
)

// A package test binary must never spawn a watcher helper: one per test would
// leave stray reasonix-desktop.exe processes behind. The host constructor is
// what the desktop calls, so it has to apply the same gate as Build.
func TestHostSkillWatchServiceIsDisabledInTestBinaries(t *testing.T) {
	if !watchSkillsEnabled() {
		if service := NewHostSkillWatchService(io.Discard); service != nil {
			_ = service.Close()
			t.Fatal("host skill watch service was created in a package test binary")
		}
		return
	}
	// A production-named binary (an installed desktop service) does own one.
	service := NewHostSkillWatchService(io.Discard)
	if service == nil {
		t.Fatal("host skill watch service is nil in a production binary")
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close host skill watch service: %v", err)
	}
}
