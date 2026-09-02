package boot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/memory"
	"reasonix/internal/provider"
)

// prefixProject builds a session in its own project and returns the system
// message that reached the provider. Every argument is a thing a project can
// differ on: its skills, its saved facts, and whether it is a repository.
func prefixProject(t *testing.T, kind, skillName, instructions string, isRepo bool, fact string) string {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &effectRecordingProvider{}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	writeFile(t, dir, "REASONIX.md", instructions)
	if isRepo {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatalf("make repository marker: %v", err)
		}
	}
	skillDir := filepath.Join(dir, ".reasonix", "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("make skill dir: %v", err)
	}
	writeFile(t, skillDir, "SKILL.md", "---\nname: "+skillName+"\ndescription: does the "+skillName+" thing\n---\n\nBody.\n")

	// The fact has to be saved by a session that then ends: a store written
	// mid-session is what the next session would have folded in.
	first, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	if _, err := first.SaveMemory(memory.Memory{
		Name: fact, Description: "a saved fact named " + fact,
		Type: memory.TypeProject, Body: "The body of " + fact + ".",
	}); err != nil {
		first.Close()
		t.Fatalf("SaveMemory: %v", err)
	}
	first.Close()

	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := agentRequests(rec.requests())
	if len(reqs) == 0 {
		t.Fatal("no agent request reached the provider boundary")
	}
	return systemOf(reqs[0])
}

// TestEffectInstructionsAreTheOnlyProjectStateInThePrefix is the standing
// boundary each per-feature effect test guards one piece of: two projects that
// share only their instructions — different skills, different saved facts, one
// a repository — must send the byte-identical system message. Instructions are
// still in the prefix, and are the next cut.
func TestEffectInstructionsAreTheOnlyProjectStateInThePrefix(t *testing.T) {
	const shared = "# Shared rules\n\nBoth projects are governed by this same file.\n"

	a := prefixProject(t, "boot-prefix-a", "alphaskill", shared, true, "deploy-target")
	b := prefixProject(t, "boot-prefix-b", "betaskill", shared, false, "release-cadence")

	if a != b {
		t.Fatalf("project state other than the instructions reached the cached prefix; every byte after it is re-sent for each project:\nfirst diff site: %q",
			firstDivergence(a, b))
	}
}
