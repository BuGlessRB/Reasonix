package telemetry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

type readinessSink struct {
	events   int
	audits   int
	recovery int
}

func (s *readinessSink) Emit(event.Event) { s.events++ }
func (s *readinessSink) RecordReadinessAudit(evidence.ReadinessAudit) {
	s.audits++
}
func (s *readinessSink) RecordProtocolRecovery(event.ProtocolRecoveryAudit) {
	s.recovery++
}

func TestSinkWritesOnlyWhitelistedContentFreeCounters(t *testing.T) {
	home := t.TempDir()
	reporter := &Reporter{
		home:    home,
		version: "v1.20.0",
		static: []Counter{
			{Signal: "client_surface", Bucket: "cli", Count: 1},
			{Signal: "cli_mode", Bucket: "run", Count: 1},
		},
	}
	inner := &readinessSink{}
	sink := reporter.Wrap(inner)
	secret := "PRIVATE_PROMPT_TOKEN_123"
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Text, Text: secret})
	sink.Emit(event.Event{Kind: event.Message, Text: secret, Reasoning: secret})
	sink.Emit(event.Event{Kind: event.Usage, Usage: &provider.Usage{
		FinishReason: "stop", CacheHitTokens: 90, CacheMissTokens: 10,
	}})
	sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
		Name: secret, Args: secret, Output: secret, Err: "permission denied: " + secret,
	}})
	event.RecordProtocolRecovery(sink, event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningRetryReplaced})
	sink.Emit(event.Event{Kind: event.TurnDone, Err: &provider.APIError{
		Provider: secret, Status: 429, Body: secret, TraceID: secret,
	}})
	event.RecordReadinessAudit(sink, evidence.ReadinessAudit{})

	entries, err := os.ReadDir(filepath.Join(home, pendingDirName))
	if err != nil || len(entries) != 1 {
		t.Fatalf("pending files = %d, err = %v", len(entries), err)
	}
	b, err := os.ReadFile(filepath.Join(home, pendingDirName, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), secret) {
		t.Fatalf("pending payload leaked private content: %s", b)
	}
	var payload pendingPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, counter := range payload.Counters {
		got[counter.Signal] = counter.Bucket
	}
	for signal, bucket := range map[string]string{
		"client_surface":               "cli",
		"cli_mode":                     "run",
		"turns":                        "count",
		"finish_reason":                "stop",
		"cache_hit":                    "90_100",
		"tool_error":                   "permission",
		"provider_error":               "rate_limit",
		"cli_exit":                     "error",
		"tool_call_reasoning_recovery": "missing_reasoning_retry_replaced_response",
	} {
		if got[signal] != bucket {
			t.Errorf("%s bucket = %q, want %q", signal, got[signal], bucket)
		}
	}
	if inner.events != 6 || inner.audits != 1 || inner.recovery != 1 {
		t.Fatalf("forwarding events=%d audits=%d recovery=%d", inner.events, inner.audits, inner.recovery)
	}
}

func TestCleanupRemovesPendingQueueOnly(t *testing.T) {
	home := t.TempDir()
	if err := appendPending(home, pendingPayload{
		Version: "v1.20.0", OS: "linux", Counters: []Counter{{Signal: "turns", Bucket: "count", Count: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	idPath := filepath.Join(home, "cli-telemetry-install-id")
	if err := os.WriteFile(idPath, []byte(strings.Repeat("a", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, pendingDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending directory still exists: %v", err)
	}
	if _, err := os.Stat(idPath); err != nil {
		t.Fatalf("install id should remain stable after opt-out cleanup: %v", err)
	}
}

func TestEnvironmentOptOutRemovesPendingQueue(t *testing.T) {
	clearPolicyEnv(t)
	home := t.TempDir()
	if err := appendPending(home, pendingPayload{
		Version: "v1.20.0", OS: "linux", Counters: []Counter{{Signal: "turns", Bucket: "count", Count: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DO_NOT_TRACK", "1")
	if reporter := Start(Options{Mode: "on", Version: "v1.20.0", HomeDir: home, Interactive: true}); reporter != nil {
		t.Fatal("environment opt-out unexpectedly started telemetry")
	}
	if _, err := os.Stat(filepath.Join(home, pendingDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment opt-out did not remove pending queue: %v", err)
	}
}

// A turn's cost is the sum of its provider requests, so the bucket has to be
// taken once at the end rather than per request — three 40k requests are one
// 120k turn, and a flat-rate plan is priced on the turn.
func TestSinkBucketsATurnsTokensOnceAcrossItsRequests(t *testing.T) {
	home := t.TempDir()
	sink := (&Reporter{home: home, version: "v1.20.0"}).Wrap(&readinessSink{})
	sink.Emit(event.Event{Kind: event.TurnStarted})
	for range 3 {
		sink.Emit(event.Event{Kind: event.Usage, Usage: &provider.Usage{
			PromptTokens: 40_000, CompletionTokens: 700, CacheHitTokens: 32_000,
		}})
	}
	sink.Emit(event.Event{Kind: event.TurnDone})

	for signal, want := range map[string]string{
		"turn_prompt_tokens": "64k_256k", // 120k, not the 40k of one request
		"turn_output_tokens": "1k_4k",    // 2.1k
		"turn_cached_tokens": "64k_256k", // 96k
	} {
		if got := onlyBucket(t, home, signal); got != want {
			t.Errorf("%s bucket = %q, want %q", signal, got, want)
		}
	}
}

// The per-turn state is grouped so both boundaries reset it wholesale. A turn
// that inherits the previous turn's tokens reports a distribution shifted right
// at every percentile, and nothing downstream could tell.
func TestSinkDoesNotCarryTokensIntoTheNextTurn(t *testing.T) {
	home := t.TempDir()
	sink := (&Reporter{home: home, version: "v1.20.0"}).Wrap(&readinessSink{})
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Usage, Usage: &provider.Usage{PromptTokens: 500_000}})
	sink.Emit(event.Event{Kind: event.TurnDone})
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.Usage, Usage: &provider.Usage{PromptTokens: 2_000}})
	sink.Emit(event.Event{Kind: event.TurnDone})

	got := pendingBuckets(t, home, "turn_prompt_tokens")
	if !slices.Contains(got, "1k_4k") {
		t.Fatalf("turn buckets = %v, want one of them %q — the first turn's 500k leaked into the second", got, "1k_4k")
	}
}

// A turn the provider never reported usage for is not absent from the
// histogram: the zero bin is what gives every other bin a denominator.
func TestSinkRecordsATurnWithNoUsageAsZero(t *testing.T) {
	home := t.TempDir()
	sink := (&Reporter{home: home, version: "v1.20.0"}).Wrap(&readinessSink{})
	sink.Emit(event.Event{Kind: event.TurnStarted})
	sink.Emit(event.Event{Kind: event.TurnDone})

	if got := onlyBucket(t, home, "turn_prompt_tokens"); got != "0" {
		t.Fatalf("bucket = %q, want %q", got, "0")
	}
}

func TestTokenBucketBoundsAreHalfOpen(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{-1, "0"}, {0, "0"}, {1, "0_1k"}, {999, "0_1k"}, {1_000, "1k_4k"},
		{255_999, "64k_256k"}, {256_000, "256k_1m"}, {999_999, "256k_1m"},
		{1_000_000, "1m_4m"}, {4_000_000, "4m_plus"},
	} {
		if got := tokenBucket(tc.n); got != tc.want {
			t.Errorf("tokenBucket(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// pendingBuckets returns the buckets recorded for one signal across every
// flushed payload. Filenames tie on the clock tick and break on a random nonce,
// so a test that reads "the last file" is a test that fails on a fast machine.
func pendingBuckets(t *testing.T, home, signal string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, pendingDirName))
	if err != nil || len(entries) == 0 {
		t.Fatalf("pending files = %d, err = %v", len(entries), err)
	}
	var out []string
	for _, entry := range entries {
		b, err := os.ReadFile(filepath.Join(home, pendingDirName, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var payload pendingPayload
		if err := json.Unmarshal(b, &payload); err != nil {
			t.Fatal(err)
		}
		for _, counter := range payload.Counters {
			if counter.Signal == signal {
				out = append(out, counter.Bucket)
			}
		}
	}
	return out
}

// only asserts the signal was recorded exactly once, then returns its bucket.
func onlyBucket(t *testing.T, home, signal string) string {
	t.Helper()
	got := pendingBuckets(t, home, signal)
	if len(got) != 1 {
		t.Fatalf("%s recorded %d times, want 1: %v", signal, len(got), got)
	}
	return got[0]
}
