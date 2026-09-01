package control

import (
	"errors"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestPinnedContextIsSeparateFromBasePrompt(t *testing.T) {
	dir := t.TempDir()
	exec := agent.New(nil, nil, agent.NewSession("stale"), agent.Options{}, event.Discard)
	ctrl := New(Options{
		Runner:        exec,
		Executor:      exec,
		SystemPrompt:  "BASE",
		PinnedContext: "PIN-A",
		SessionDir:    dir,
		SessionPath:   filepath.Join(dir, "session.jsonl"),
		Sink:          event.Discard,
	})
	if got := controlSystemMessage(ctrl.History()); got != "BASE\n\nPIN-A" {
		t.Fatalf("initial system prompt = %q", got)
	}

	ctrl.ApplyExtensionSystemPrompt("EXTENSION")
	if got := controlSystemMessage(ctrl.History()); got != "EXTENSION\n\nPIN-A" {
		t.Fatalf("extension prompt dropped or duplicated pins: %q", got)
	}
	if err := ctrl.SetPinnedContext("PIN-B"); err != nil {
		t.Fatalf("SetPinnedContext: %v", err)
	}
	if got := controlSystemMessage(ctrl.History()); got != "EXTENSION\n\nPIN-B" {
		t.Fatalf("updated system prompt = %q", got)
	}

	ctrl.mu.Lock()
	ctrl.running = true
	ctrl.mu.Unlock()
	if err := ctrl.SetPinnedContext("PIN-C"); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("running SetPinnedContext = %v, want ErrTurnRunning", err)
	}
	ctrl.mu.Lock()
	ctrl.running = false
	ctrl.mu.Unlock()
	if got := controlSystemMessage(ctrl.History()); got != "EXTENSION\n\nPIN-B" {
		t.Fatalf("busy mutation changed prompt: %q", got)
	}

	if err := ctrl.NewSession(); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if got := controlSystemMessage(ctrl.History()); got != "EXTENSION" {
		t.Fatalf("new session inherited pins: %q", got)
	}
}

func TestPinnedContextDerivesOmittedBaseFromExecutorSession(t *testing.T) {
	exec := agent.New(nil, nil, agent.NewSession("MODEL CONTRACT"), agent.Options{}, event.Discard)
	ctrl := New(Options{Executor: exec, PinnedContext: "PIN", Sink: event.Discard})
	if got := controlSystemMessage(ctrl.History()); got != "MODEL CONTRACT\n\nPIN" {
		t.Fatalf("derived prompt = %q", got)
	}
	if err := ctrl.SetPinnedContext(""); err != nil {
		t.Fatal(err)
	}
	if got := controlSystemMessage(ctrl.History()); got != "MODEL CONTRACT" {
		t.Fatalf("clearing pins lost derived base prompt: %q", got)
	}
}

func TestSessionTransitionPublishesTargetPinnedContext(t *testing.T) {
	exec := agent.New(nil, nil, agent.NewSession("BASE\n\nSOURCE"), agent.Options{}, event.Discard)
	ctrl := New(Options{Runner: exec, Executor: exec, SystemPrompt: "BASE", PinnedContext: "SOURCE", Sink: event.Discard})
	target := agent.NewSession("persisted-old-system")
	target.Add(provider.Message{Role: provider.RoleUser, Content: "history"})
	commit, err := ctrl.prepareSessionTransition("target.jsonl", "switch", target)
	if err != nil {
		t.Fatalf("prepareSessionTransition: %v", err)
	}
	info := SessionTransitionInfo{TargetPath: "target.jsonl", session: target, commit: commit}
	info.SetPinnedContext("TARGET")
	commit.publish()

	if got := ctrl.SystemPrompt(); got != "BASE\n\nTARGET" {
		t.Fatalf("controller prompt = %q", got)
	}
	if got := controlSystemMessage(ctrl.History()); got != "BASE\n\nTARGET" {
		t.Fatalf("published candidate prompt = %q", got)
	}
}
