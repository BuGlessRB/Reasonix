package boot

// Completion-validator effect tests: the evaluator's isolated request shape at
// the provider boundary, and main-request byte stability across modes.

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/completioneval"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// verdictProvider answers the completion validator's request with a fixed
// verdict and every other request with a plain final answer.
type verdictProvider struct {
	effectRecordingProvider
	verdict string
}

func (p *verdictProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	verdict := p.verdict
	p.mu.Unlock()
	text := "ok"
	if len(req.Messages) > 0 && req.Messages[0].Role == provider.RoleSystem &&
		req.Messages[0].Content == completioneval.PolicyPrompt {
		text = verdict
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: text}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// effectRunWithAgentConfig builds the real stack with extra [agent] TOML lines
// and runs one prompt; runErr reports the controller's run result.
func effectRunWithAgentConfig(t *testing.T, kind string, rec provider.Provider, agentLines string) []provider.Request {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	provider.Register(kind, func(provider.Config) (provider.Provider, error) {
		return rec, nil
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
`+agentLines+`

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := rec.(*verdictProvider).requests()
	if len(reqs) == 0 {
		t.Fatal("no request reached the provider boundary")
	}
	return reqs
}

func mainSystem(req provider.Request) string {
	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem {
			return m.Content
		}
	}
	return ""
}

func splitValidatorRequests(reqs []provider.Request) (main, validator []provider.Request) {
	for _, req := range reqs {
		if len(req.Messages) > 0 && req.Messages[0].Role == provider.RoleSystem &&
			req.Messages[0].Content == completioneval.PolicyPrompt {
			validator = append(validator, req)
		} else {
			main = append(main, req)
		}
	}
	return main, validator
}

// The unconfigured default is shadow: the validator's request really reaches
// the provider boundary, tool-less and policy-isolated, while the main run
// still succeeds.
func TestEffectCompletionValidatorShadowDefaultReachesProvider(t *testing.T) {
	rec := &verdictProvider{verdict: `{"outcome":"continue","reason":"shadow record only"}`}
	reqs := effectRunWithAgentConfig(t, "boot-effect-cv-shadow", rec, "")
	main, validator := splitValidatorRequests(reqs)
	if len(validator) != 1 {
		t.Fatalf("validator requests = %d, want 1 in shadow default (got %d total)", len(validator), len(reqs))
	}
	v := validator[0]
	if len(v.Tools) != 0 {
		t.Fatalf("validator request carries %d tool schemas; it must be tool-less", len(v.Tools))
	}
	if len(v.Messages) != 2 || v.Messages[1].Role != provider.RoleUser {
		t.Fatalf("validator request messages = %d roles; want exactly policy+evidence", len(v.Messages))
	}
	if v.MaxTokens != completioneval.MaxTokens {
		t.Fatalf("validator MaxTokens = %d, want %d", v.MaxTokens, completioneval.MaxTokens)
	}
	if !strings.Contains(v.Messages[1].Content, `"candidate_answer"`) {
		t.Fatalf("validator evidence missing candidate answer: %s", v.Messages[1].Content)
	}
	if strings.Contains(v.Messages[1].Content, "reply ok") == false {
		t.Fatal("validator evidence missing the task text")
	}
	if len(main) == 0 {
		t.Fatal("main conversation request missing")
	}
}

// An explicit off keeps the validator off the wire entirely.
func TestEffectCompletionValidatorOffSendsNothing(t *testing.T) {
	rec := &verdictProvider{verdict: `{"outcome":"complete"}`}
	reqs := effectRunWithAgentConfig(t, "boot-effect-cv-off", rec, `completion_validation = "off"`)
	_, validator := splitValidatorRequests(reqs)
	if len(validator) != 0 {
		t.Fatalf("validator requests = %d, want 0 when off", len(validator))
	}
}

// Enforce with a confirming verdict completes normally through the real stack.
func TestEffectCompletionValidatorEnforceCompleteFinishes(t *testing.T) {
	rec := &verdictProvider{verdict: `{"outcome":"complete"}`}
	reqs := effectRunWithAgentConfig(t, "boot-effect-cv-enforce", rec, `completion_validation = "enforce"`)
	_, validator := splitValidatorRequests(reqs)
	if len(validator) != 1 {
		t.Fatalf("validator requests = %d, want 1", len(validator))
	}
}

// The main conversation's request bytes must not move with the validator mode:
// same system prompt, same tool schemas, same message shape. This is the
// prompt-cache contract at the boundary the cache actually keys on. Both
// builds share one directory so workspace paths in the prompt stay identical.
func TestEffectMainRequestStableAcrossValidatorModes(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	run := func(kind, agentLine string) []provider.Request {
		rec := &verdictProvider{verdict: `{"outcome":"complete"}`}
		provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
		writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
`+agentLine+`

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
		ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		defer ctrl.Close()
		if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return rec.requests()
	}
	offReqs := run("boot-effect-cv-stab-off", `completion_validation = "off"`)
	enforceReqs := run("boot-effect-cv-stab-enforce", `completion_validation = "enforce"`)

	offMain, _ := splitValidatorRequests(offReqs)
	enforceMain, _ := splitValidatorRequests(enforceReqs)
	if len(offMain) == 0 || len(enforceMain) == 0 {
		t.Fatalf("main requests: off=%d enforce=%d", len(offMain), len(enforceMain))
	}
	offSystem, enforceSystem := mainSystem(offMain[0]), mainSystem(enforceMain[0])
	if offSystem != enforceSystem {
		t.Fatalf("system prompt changed with validator mode:\noff=%q\nenforce=%q", offSystem, enforceSystem)
	}
	if !strings.HasPrefix(offSystem, "BASE") {
		t.Fatalf("system prompt = %q, want the configured BASE prompt prefix", offSystem[:min(80, len(offSystem))])
	}
	if len(enforceMain[0].Tools) != len(offMain[0].Tools) {
		t.Fatalf("tool schema count changed with validator mode: off=%d enforce=%d", len(offMain[0].Tools), len(enforceMain[0].Tools))
	}
	for i := range offMain[0].Tools {
		if offMain[0].Tools[i].Name != enforceMain[0].Tools[i].Name {
			t.Fatalf("tool order changed at %d: off=%s enforce=%s", i, offMain[0].Tools[i].Name, enforceMain[0].Tools[i].Name)
		}
	}
}
