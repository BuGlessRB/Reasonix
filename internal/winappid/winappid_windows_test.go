//go:build windows

package winappid

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetShortcutIDMissingFile(t *testing.T) {
	err := setShortcutID(filepath.Join(t.TempDir(), "nope.lnk"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v; want os.ErrNotExist", err)
	}
}

func TestSetShortcutIDSkipsNonRegular(t *testing.T) {
	lnk := filepath.Join(t.TempDir(), "Reasonix.exe.lnk")
	if err := os.Mkdir(lnk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := setShortcutID(lnk); err != nil {
		t.Fatalf("setShortcutID on directory: %v", err)
	}
}

func TestEnsureShortcutIDsStampsInOrder(t *testing.T) {
	taskbar := t.TempDir()
	if err := os.WriteFile(filepath.Join(taskbar, "Reasonix.exe.lnk"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, origSetter := windowsPinnedTaskbarDir, windowsShortcutSetter
	defer func() { windowsPinnedTaskbarDir, windowsShortcutSetter = origDir, origSetter }()
	windowsPinnedTaskbarDir = func() (string, error) { return taskbar, nil }
	var stamped []string
	windowsShortcutSetter = func(path string) error {
		stamped = append(stamped, path)
		return nil
	}

	if err := EnsureShortcutIDs(); err != nil {
		t.Fatalf("EnsureShortcutIDs: %v", err)
	}
	// The injected setter records every candidate in order; the real setter
	// filters missing/dangling entries itself (see TestSetShortcutID*).
	want := []string{
		filepath.Join(taskbar, "Reasonix.exe.lnk"),
		filepath.Join(taskbar, "reasonix-launcher.exe.lnk"),
		filepath.Join(taskbar, "reasonix-desktop.exe.lnk"),
	}
	if len(stamped) != len(want) {
		t.Fatalf("stamped = %v; want %v", stamped, want)
	}
	for i := range want {
		if stamped[i] != want[i] {
			t.Fatalf("stamped[%d] = %q; want %q", i, stamped[i], want[i])
		}
	}
}

func TestEnsureShortcutIDsNoPinnedFolder(t *testing.T) {
	origDir, origSetter := windowsPinnedTaskbarDir, windowsShortcutSetter
	defer func() { windowsPinnedTaskbarDir, windowsShortcutSetter = origDir, origSetter }()
	windowsPinnedTaskbarDir = func() (string, error) {
		return "", os.ErrNotExist
	}
	called := false
	windowsShortcutSetter = func(string) error {
		called = true
		return nil
	}
	if err := EnsureShortcutIDs(); err != nil {
		t.Fatalf("EnsureShortcutIDs: %v", err)
	}
	if called {
		t.Fatal("setter called although no pinned folder exists")
	}
}

func TestEnsureShortcutIDsJoinsErrors(t *testing.T) {
	taskbar := t.TempDir()
	good := filepath.Join(taskbar, "Reasonix.exe.lnk")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, origSetter := windowsPinnedTaskbarDir, windowsShortcutSetter
	defer func() { windowsPinnedTaskbarDir, windowsShortcutSetter = origDir, origSetter }()
	windowsPinnedTaskbarDir = func() (string, error) { return taskbar, nil }
	windowsShortcutSetter = func(path string) error {
		if path == good {
			return nil
		}
		return errors.New("boom")
	}
	if err := EnsureShortcutIDs(); err == nil {
		t.Fatal("EnsureShortcutIDs: want joined error, got nil")
	}
}
