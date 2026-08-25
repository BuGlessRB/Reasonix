package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Where a Studio crash goes to not be lost. Every one reported so far has been
// a fatal signal inside GTK or WebKit, and the runtime writes that trace to the
// descriptor — not through a writer this process could tee. Launched from its
// .desktop entry nothing holds the other end, which is why the only trace ever
// attached to an issue came from someone who started Studio from a shell.
const (
	studioLogPrefix = "studio-"
	studioLogsKept  = 5
)

// openCrashLog points this process's stderr at a file under <home>/logs and
// returns where the window's own logs should go: that file, plus the terminal
// when there was one, so running Studio from a shell still prints.
func openCrashLog(home string) (io.Writer, func(), error) {
	quiet := func() {}
	if strings.TrimSpace(home) == "" {
		return os.Stderr, quiet, nil
	}
	dir := filepath.Join(home, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return os.Stderr, quiet, err
	}
	name := fmt.Sprintf("%s%s-%d.log", studioLogPrefix, time.Now().UTC().Format("20060102-150405"), os.Getpid())
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return os.Stderr, quiet, err
	}
	pruneStudioLogs(dir)
	flush := func() { _ = file.Sync(); _ = file.Close() }

	if !canRedirectStderr {
		// The window's own logs still reach the file; a fatal signal's trace
		// does not. Windows resolves its stderr handle once at start, so moving
		// os.Stderr afterwards would not move the write that matters.
		return io.MultiWriter(os.Stderr, file), flush, nil
	}
	// Taken before the redirect: afterwards fd 2 is the file, and there is no
	// way back to what it used to be.
	tty := terminalStderr()
	if err := redirectStderr(file); err != nil {
		return io.MultiWriter(os.Stderr, file), flush, err
	}
	if tty == nil {
		return os.Stderr, flush, nil
	}
	return io.MultiWriter(os.Stderr, tty), flush, nil
}

// terminalStderr duplicates stderr while it still is one, and answers nil when
// this process was launched without a terminal — from a .desktop entry, which
// is how the people who hit these crashes start it.
func terminalStderr() *os.File {
	info, err := os.Stderr.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	dup, err := dupStderr()
	if err != nil {
		return nil
	}
	return dup
}

// pruneStudioLogs keeps the newest few. One log per launch is what makes the
// run before this one still readable — a crash is reported after a relaunch,
// never during the run that produced it.
func pruneStudioLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var logs []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, studioLogPrefix) && strings.HasSuffix(name, ".log") {
			logs = append(logs, name)
		}
	}
	// The name carries the stamp, so sorting the name sorts by age.
	sort.Sort(sort.Reverse(sort.StringSlice(logs)))
	for _, name := range logs[min(len(logs), studioLogsKept):] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
