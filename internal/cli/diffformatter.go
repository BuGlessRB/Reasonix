// This file implements the optional external diff formatter: a user-global
// command (e.g. delta) that formats a diff before the CLI emits it — both a
// fenced ```diff / ```patch block in the conversation answer stream and a writer
// tool's diff card in the transcript. Configured via [cli].diff_formatter as an
// argv line and exec'd directly — no shell.
package cli

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"reasonix/internal/config"
)

const (
	// diffFormatTimeout bounds the external formatter so a hung command cannot
	// stall the render; on timeout the block falls back to the built-in path.
	diffFormatTimeout = 3 * time.Second
	// diffFormatMaxBytes skips the external formatter for very large blocks,
	// where the subprocess round-trip costs more than it saves and the built-in
	// renderer (which folds) is the better default.
	diffFormatMaxBytes = 1 << 20 // 1 MiB
)

// activeDiffFormatter is the argv of the configured [cli].diff_formatter; empty
// when unset. Resolved once at CLI startup, like activeCLITheme.
var activeDiffFormatter []string

// configureDiffFormatter resolves [cli].diff_formatter into activeDiffFormatter.
// It is user-global only: a repo-local reasonix.toml cannot run a command on the
// user's machine.
func configureDiffFormatter(cfg *config.Config) {
	activeDiffFormatter = nil
	if cfg == nil {
		return
	}
	activeDiffFormatter = splitDiffFormatter(cfg.CLI.DiffFormatter)
}

// splitDiffFormatter splits a configured command line into argv, honouring single
// and double quotes so a path with spaces survives. It performs no shell
// expansion — the argv is exec'd directly.
func splitDiffFormatter(s string) []string {
	var argv []string
	var cur strings.Builder
	haveToken := false
	var quote byte // 0 = none, '\'' or '"'
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			haveToken = true
		case c == ' ' || c == '\t':
			if haveToken {
				argv = append(argv, cur.String())
				cur.Reset()
				haveToken = false
			}
		default:
			cur.WriteByte(c)
			haveToken = true
		}
	}
	if haveToken {
		argv = append(argv, cur.String())
	}
	return argv
}

// renderDiffExternal pipes the unified diff through argv (exec, no shell) and
// returns its stdout. ok is false when the command is empty, the diff is too
// large, the run fails, or it times out — callers then keep the built-in
// renderer.
func renderDiffExternal(argv []string, diff string) (string, bool) {
	if len(argv) == 0 || diff == "" || len(diff) > diffFormatMaxBytes {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), diffFormatTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(diff)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return "", false
	}
	return out.String(), true
}

// formatToolDiff pipes a writer call's unified diff through the configured
// [cli].diff_formatter and returns its stdout as transcript rows, indented to
// the card's body column. ok is false when no formatter is configured or the
// run fails, times out, or overshoots the size cap — the caller then keeps the
// built-in renderer. Mirroring the fenced-diff path, the output is shown
// verbatim: no gutter, no width clamp, no escape filtering, and no fold, so the
// formatter's own colours and layout reach the transcript intact.
func formatToolDiff(diff string) ([]string, bool) {
	out, ok := renderDiffExternal(activeDiffFormatter, diff)
	if !ok {
		return nil, false
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	rows := make([]string, 0, len(lines))
	for _, ln := range lines {
		rows = append(rows, "  "+ln)
	}
	return rows, true
}
