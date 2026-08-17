package agent

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

// Measured against the real triage model: a prompt that reads well is not
// evidence that it answers well. Skipped in CI — needs network and a key.
// Run with REASONIX_LIVE_TRIAGE=1.
const verificationClassSystemPrompt = `You classify one shell command as a correctness check or not.

CHECK: running it decides whether code is correct or well-formed and reports the
verdict through its exit status — a test run, a type check, a linter or static
analyser, a compile whose only purpose is to see whether it compiles, or a
project script that runs those.

NOT: it changes code or files; it formats or fixes in place; it only lists,
collects or plans work without running it; it prints information; it installs,
builds an artifact, or manages the environment.

Judge the command in front of you, not the program in general. A runner told to
select nothing checks nothing: -run with a pattern that matches no test,
--collect-only, --dry-run, --list, -h. A command that writes what it inspects
(--fix, --write, -w, -i, --update, a > redirection) is NOT a check.

Answer with exactly one word: CHECK or NOT.
If you cannot tell what it would do, answer NOT.`

type classCase struct {
	command string
	check   bool   // what a correct classifier answers
	why     string // what this case is here to prove
}

// The set is chosen around the two boundaries that matter: commands the static
// table cannot name (the reason to escalate at all), and commands that look
// like checks but run none (the reason a table is not enough).
var verificationClassCases = []classCase{
	{"./scripts/verify.sh", true, "project script — the whole point"},
	{"make smoke", true, "project target the table does not list"},
	{"bash scripts/ci.sh", true, "script behind an interpreter"},
	{"npm run e2e", true, "script name outside the table's four"},
	{"tox -e py311", true, "runner the table never heard of"},
	{"bazel test //...", true, "runner the table never heard of"},
	{"ctest --output-on-failure", true, "runner the table never heard of"},
	{"go test -run NoSuchTestName ./...", false, "runs nothing, exits zero — the table accepts this today"},
	{"pytest --collect-only", false, "collects without running"},
	{"go build -o app ./cmd/app", false, "produces an artifact"},
	{"gofmt -w .", false, "rewrites in place"},
	{"npm install", false, "changes the environment"},
	{"cat README.md", false, "prints information"},
	{"git status", false, "prints information"},
	{"rm -rf build", false, "destroys"},
	{"./scripts/deploy.sh", false, "a script, but not a check"},
}

func TestVerificationClassLive(t *testing.T) {
	if os.Getenv("REASONIX_LIVE_TRIAGE") != "1" {
		t.Skip("set REASONIX_LIVE_TRIAGE=1 to measure the classifier against the real model")
	}
	prov, ref := liveTriageProvider(t)
	agent := &Agent{}
	agent.svc.triage = prov

	var wrong, agreed int
	for _, tc := range verificationClassCases {
		ctx, cancel := context.WithTimeout(context.Background(), commandClassTimeout)
		got := agent.askClass(ctx, verificationClassSystemPrompt, tc.command, "CHECK")
		cancel()

		// What the static table would have said on its own, so the measurement
		// shows what the escalation adds rather than only how it scores.
		static := evidence.CommandRunsVerification(tc.command)
		if static == tc.check {
			agreed++
		}
		mark := "ok "
		if got != tc.check {
			mark = "MISS"
			wrong++
		}
		t.Logf("%s  model=%-5v want=%-5v static=%-5v  %s   (%s)",
			mark, got, tc.check, static, tc.command, tc.why)
	}
	total := len(verificationClassCases)
	t.Logf("model %s: %d/%d correct; the static table alone: %d/%d",
		ref, total-wrong, total, agreed, total)
	if wrong*4 > total {
		t.Errorf("classifier missed %d of %d — worse than a prompt this narrow should be", wrong, total)
	}
}

// liveTriageProvider builds the triage model from the user's own config, so the
// measurement runs against what the escalation would really use. No key is
// handled here: config resolves it the way every other surface does.
func liveTriageProvider(t *testing.T) (provider.Provider, string) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("no usable config: %v", err)
	}
	ref := strings.TrimSpace(cfg.Agent.TriageModel)
	for _, fallback := range []string{cfg.Agent.SubagentModel, cfg.DefaultModel} {
		if ref != "" {
			break
		}
		ref = strings.TrimSpace(fallback)
	}
	if ref == "" {
		t.Skip("no model configured to triage with")
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok {
		t.Skipf("configured triage model %q does not resolve", ref)
	}
	if entry.RequiresAPIKey() && strings.TrimSpace(entry.APIKey()) == "" {
		t.Skipf("no key configured for %q", ref)
	}
	prov, err := provider.New(entry.Kind, provider.Config{
		Name: entry.Name, BaseURL: entry.BaseURL, Model: entry.Model, APIKey: entry.APIKey(),
		Extra: map[string]any{
			"api_key_env":        entry.APIKeyEnv,
			"thinking":           entry.Thinking,
			"reasoning_protocol": config.ReasoningProtocolForEntry(entry),
		},
	})
	if err != nil {
		t.Skipf("cannot build %q: %v", ref, err)
	}
	// The package verifies against goroutine leaks, and a keep-alive HTTP/2
	// connection outlives the call that opened it. Real traffic is the point of
	// this test, so it closes what it opened rather than the package relaxing.
	t.Cleanup(func() {
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
		if closer, ok := prov.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	})
	return prov, ref
}
