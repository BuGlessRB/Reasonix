package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The point of the redirect is the trace the Go runtime writes straight to the
// descriptor when a fatal signal lands — every Studio crash reported so far has
// been one, inside GTK or WebKit, where no recover can reach. A tee on a writer
// would keep every log line and lose exactly that, so what this asserts is the
// descriptor and not the writer.
func TestCrashLogCatchesWhatIsWrittenToTheDescriptor(t *testing.T) {
	if !canRedirectStderr {
		t.Skip("this platform keeps the handle its crash writer resolved at start")
	}
	saved, err := dupStderr()
	if err != nil {
		t.Fatalf("dup stderr: %v", err)
	}
	t.Cleanup(func() {
		_ = redirectStderr(saved)
		_ = saved.Close()
	})

	home := t.TempDir()
	logs, flush, err := openCrashLog(home)
	if err != nil {
		t.Fatalf("openCrashLog: %v", err)
	}
	fmt.Fprintln(os.Stderr, "FATAL-TRACE-STANDIN")
	fmt.Fprintln(logs, "WINDOW-LOG-LINE")
	flush()

	body := theOnlyStudioLog(t, home)
	for _, want := range []string{"FATAL-TRACE-STANDIN", "WINDOW-LOG-LINE"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the log is missing %q; it holds:\n%s", want, body)
		}
	}
}

// The other half, and the only half a platform without the redirect gets: what
// the window itself logs has to reach the file on every platform, or a report
// arrives with the crash and nothing that led up to it.
func TestCrashLogKeepsWhatTheWindowLogs(t *testing.T) {
	saved := (*os.File)(nil)
	if canRedirectStderr {
		var err error
		if saved, err = dupStderr(); err != nil {
			t.Fatalf("dup stderr: %v", err)
		}
		t.Cleanup(func() {
			_ = redirectStderr(saved)
			_ = saved.Close()
		})
	}

	home := t.TempDir()
	logs, flush, err := openCrashLog(home)
	if err != nil {
		t.Fatalf("openCrashLog: %v", err)
	}
	fmt.Fprintln(logs, "WINDOW-LOG-LINE")
	flush()

	if body := theOnlyStudioLog(t, home); !strings.Contains(body, "WINDOW-LOG-LINE") {
		t.Fatalf("the log is missing the window's own line; it holds:\n%s", body)
	}
}

// A home Studio cannot write to is not a reason not to start: the window is
// what the person came for, and the log is what would have helped later.
func TestCrashLogWithoutAHomeStillAnswersAWriter(t *testing.T) {
	logs, flush, err := openCrashLog("")
	defer flush()
	if err != nil {
		t.Fatalf("openCrashLog: %v", err)
	}
	if logs == nil {
		t.Fatal("a window with nowhere to log still has to have somewhere to write")
	}
}

func theOnlyStudioLog(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, "logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("want one log, got %v", names)
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A crash is reported after a relaunch, never during the run that produced it,
// so the log worth keeping is never the newest one.
func TestPruneStudioLogsKeepsTheNewestFew(t *testing.T) {
	dir := t.TempDir()
	var made []string
	for i := range studioLogsKept + 3 {
		name := fmt.Sprintf("%s2026010%d-000000-%d.log", studioLogPrefix, i, i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		made = append(made, name)
	}
	// The directory is shared with the CLI's own diagnostics, so anything this
	// file does not own has to survive.
	const other = "cli-tui-20260101-000000.log"
	if err := os.WriteFile(filepath.Join(dir, other), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	pruneStudioLogs(dir)

	left := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		left[entry.Name()] = true
	}
	if !left[other] {
		t.Fatal("pruning took a log this file does not own")
	}
	for _, name := range made[:3] {
		if left[name] {
			t.Fatalf("%s should have been pruned; left = %v", name, left)
		}
	}
	for _, name := range made[3:] {
		if !left[name] {
			t.Fatalf("%s should have been kept; left = %v", name, left)
		}
	}
}
