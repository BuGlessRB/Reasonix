package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/provider"
)

type pinnedPromptProvider struct {
	requests chan provider.Request
}

func (p *pinnedPromptProvider) Name() string { return "pinned-prompt" }

func (p *pinnedPromptProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.requests <- req
	chunks := make(chan provider.Chunk, 2)
	chunks <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	chunks <- provider.Chunk{Type: provider.ChunkDone}
	close(chunks)
	return chunks, nil
}

func submitAndCapturePinnedPrompt(t *testing.T, app *App, tab *WorkspaceTab, ctrl *control.Controller, requests <-chan provider.Request, input string) string {
	t.Helper()
	if err := app.SubmitToTab(tab.ID, input); err != nil {
		t.Fatalf("SubmitToTab(%q): %v", input, err)
	}
	var req provider.Request
	select {
	case req = <-requests:
	case <-time.After(5 * time.Second):
		t.Fatalf("provider did not receive %q", input)
	}
	deadline := time.Now().Add(5 * time.Second)
	for ctrl.RuntimeStatus().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ctrl.RuntimeStatus().Running {
		t.Fatalf("turn %q did not finish", input)
	}
	if len(req.Messages) == 0 || req.Messages[0].Role != provider.RoleSystem {
		t.Fatalf("request %q has no leading system message: %+v", input, req.Messages)
	}
	return req.Messages[0].Content
}

func TestDesktopPinAPIRefreshesProviderPrefixOnlyWhenFileChanges(t *testing.T) {
	prov := &pinnedPromptProvider{requests: make(chan provider.Request, 4)}
	app, tab, ctrl, _ := pinnedConcurrencyFixture(t, prov)
	path := filepath.Join(tab.WorkspaceRoot, "context.md")
	if err := os.WriteFile(path, []byte("version one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PinFileForTab(tab.ID, "context.md"); err != nil {
		t.Fatalf("PinFileForTab: %v", err)
	}

	first := submitAndCapturePinnedPrompt(t, app, tab, ctrl, prov.requests, "first")
	second := submitAndCapturePinnedPrompt(t, app, tab, ctrl, prov.requests, "second")
	if second != first {
		t.Fatalf("unchanged pinned file changed provider prefix:\nfirst: %q\nsecond: %q", first, second)
	}
	if err := os.WriteFile(path, []byte("version two"), 0o600); err != nil {
		t.Fatal(err)
	}
	third := submitAndCapturePinnedPrompt(t, app, tab, ctrl, prov.requests, "third")
	if third == second || !strings.Contains(third, "version two") {
		t.Fatalf("changed pinned file did not refresh provider prefix: %q", third)
	}
	fourth := submitAndCapturePinnedPrompt(t, app, tab, ctrl, prov.requests, "fourth")
	if fourth != third {
		t.Fatalf("stable post-refresh prefix drifted:\nthird: %q\nfourth: %q", third, fourth)
	}
}
