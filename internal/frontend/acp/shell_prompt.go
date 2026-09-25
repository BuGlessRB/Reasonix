package acp

import (
	"context"
	"strings"
	"time"
)

// shellPollInterval is how often a `!` prompt looks again at whether the
// command it started has finished.
const shellPollInterval = 20 * time.Millisecond

// runPrompt runs one prompt: a `!` line is the user's own shell command, run
// directly and streamed back as a tool call; anything else is a model turn.
// The editor is a frontend in the kernel's process, so it holds that grant.
func runPrompt(ctx context.Context, ctrl acpController, text string) error {
	cmd, ok := strings.CutPrefix(strings.TrimSpace(text), "!")
	if !ok {
		return ctrl.RunTurn(ctx, text)
	}
	ctrl.RunShell(cmd)
	tick := time.NewTicker(shellPollInterval)
	defer tick.Stop()
	for ctrl.Running() {
		select {
		case <-ctx.Done():
			ctrl.Cancel()
			return ctx.Err()
		case <-tick.C:
		}
	}
	return nil
}
