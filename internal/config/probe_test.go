package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// modelsServer answers GET .../models the way one protocol's gateway does, and
// 401s every other auth shape — which is how the probe tells them apart.
func modelsServer(t *testing.T, wantAuth func(*http.Request) bool, ids ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			http.NotFound(w, r)
			return
		}
		if !wantAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body := struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}{}
		for _, id := range ids {
			body.Data = append(body.Data, struct {
				ID string `json:"id"`
			}{ID: id})
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func bearer(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer k" }
func xAPIKey(r *http.Request) bool {
	return r.Header.Get("x-api-key") == "k" && r.Header.Get("Authorization") == ""
}

func TestProbeIdentifiesAnOpenAICompatibleEndpoint(t *testing.T) {
	srv := modelsServer(t, bearer, "kimi-k2", "moonshot-v1-8k")
	got, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL + "/v1", APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if got.Kind != "openai" || got.AuthHeader {
		t.Fatalf("kind=%q authHeader=%v, want openai without the Bearer override", got.Kind, got.AuthHeader)
	}
	if !slices.Contains(got.Models, "kimi-k2") {
		t.Fatalf("models = %v, want the listing", got.Models)
	}
	if got.Default != got.Models[0] {
		t.Fatalf("default = %q, want the first model %q", got.Default, got.Models[0])
	}
}

// An Anthropic-compatible gateway is told apart by which auth shape it accepts,
// not by its URL — the same host can serve either.
func TestProbeIdentifiesAnAnthropicEndpoint(t *testing.T) {
	srv := modelsServer(t, xAPIKey, "claude-opus-4-8")
	got, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if got.Kind != "anthropic" || got.AuthHeader {
		t.Fatalf("kind=%q authHeader=%v, want anthropic on x-api-key", got.Kind, got.AuthHeader)
	}
}

// A gateway that takes Bearer answers both listings identically, so the model
// list cannot separate them. The probe must say it is unsure rather than
// quietly calling every such endpoint OpenAI.
func TestProbeAdmitsWhenBothProtocolsAnswer(t *testing.T) {
	srv := modelsServer(t, bearer, "MiniMax-M2")
	got, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if !got.Ambiguous {
		t.Fatal("two protocols answered but the probe reported a confident guess")
	}
	// A single-protocol endpoint must not be marked unsure, or the warning
	// becomes noise the user learns to click past.
	only := modelsServer(t, xAPIKey, "claude-opus-4-8")
	got, err = ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: only.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if got.Ambiguous {
		t.Fatalf("only x-api-key answered, but the probe reported ambiguity: %+v", got)
	}
}

// The route says what it carries: MiniMax Global and Vercel's gateway put
// Anthropic traffic behind an /anthropic path and take Bearer. That path is the
// strongest evidence available, and it is the same knowledge the model-fetch
// compat suffixes already encode.
func TestProbePrefersAnthropicWhenThePathSaysSo(t *testing.T) {
	srv := modelsServer(t, bearer, "MiniMax-M2")
	got, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL + "/anthropic", APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if got.Kind != "anthropic" || !got.AuthHeader {
		t.Fatalf("kind=%q authHeader=%v, want anthropic with the Bearer override", got.Kind, got.AuthHeader)
	}
}

// Claude ids are the other evidence: a listing that is all Claude is not an
// OpenAI catalog, whatever auth it happens to accept.
func TestProbePrefersAnthropicForAClaudeOnlyCatalog(t *testing.T) {
	srv := modelsServer(t, bearer, "claude-opus-4-8", "claude-haiku-4-5")
	got, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if got.Kind != "anthropic" {
		t.Fatalf("kind = %q for an all-Claude catalog, want anthropic", got.Kind)
	}
}

// A listing with nothing conversational in it is not a usable provider, and
// saying so beats adding one whose model picker is empty.
func TestProbeRejectsAnEndpointWithNoChatModels(t *testing.T) {
	srv := modelsServer(t, bearer, "text-embedding-3-large", "whisper-1", "bge-reranker-v2")
	_, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL, APIKey: "k"})
	if err == nil || !strings.Contains(err.Error(), "chat models") {
		t.Fatalf("error = %v, want it to name the missing chat models", err)
	}
}

func TestProbeReportsAnEndpointThatAnswersNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL, APIKey: "k"}); err == nil {
		t.Fatal("an endpoint that lists nothing must not probe clean")
	}
	if _, err := ProbeEndpoint(context.Background(), ProbeOptions{APIKey: "k"}); err == nil {
		t.Fatal("an empty address must be refused before any request")
	}
}

// What the panel shows has to be what the runtime will do, so the capability
// fields come from the same registries a saved provider is read through.
func TestProbeReadsCapabilitiesFromTheSameRegistries(t *testing.T) {
	srv := modelsServer(t, bearer, "deepseek-v4-flash", "some-vl-model")
	got, err := ProbeEndpoint(context.Background(), ProbeOptions{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	want := EffortCapabilityForEntry(&ProviderEntry{Kind: "openai", Model: "deepseek-v4-flash"})
	if !slices.Equal(got.Efforts, want.Levels) || got.Effort != want.Default {
		t.Fatalf("efforts = %v/%q, want %v/%q from the registry", got.Efforts, got.Effort, want.Levels, want.Default)
	}
	if !slices.Equal(got.Vision, InferVisionModels(got.Models)) {
		t.Fatalf("vision = %v, want the inferred set", got.Vision)
	}
}

// A China-only endpoint reached through a foreign exit fails the same way a
// wrong address does (#2803). Retrying without the proxy is what turns "your
// endpoint is wrong" into a provider that works with no_proxy set.
func TestProbeFallsBackToADirectRouteAndSaysSo(t *testing.T) {
	srv := modelsServer(t, bearer, "kimi-k2")
	dead := &http.Client{Transport: rtErr{}}

	got, err := ProbeEndpoint(context.Background(), ProbeOptions{
		BaseURL: srv.URL, APIKey: "k", Client: dead, Direct: srv.Client(),
	})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if !got.NoProxy {
		t.Fatal("the endpoint answered only with the proxy bypassed, but no_proxy was not reported")
	}
	// An endpoint the proxy can reach must not be marked no_proxy, or every
	// provider would quietly opt out of a proxy the user needs.
	got, err = ProbeEndpoint(context.Background(), ProbeOptions{
		BaseURL: srv.URL, APIKey: "k", Client: srv.Client(), Direct: srv.Client(),
	})
	if err != nil {
		t.Fatalf("ProbeEndpoint: %v", err)
	}
	if got.NoProxy {
		t.Fatal("the proxy reached the endpoint, but the probe still asked to bypass it")
	}
	// When neither route works, the proxied failure is what the user sees —
	// it is the route their other providers use.
	if _, err = ProbeEndpoint(context.Background(), ProbeOptions{
		BaseURL: srv.URL, APIKey: "k", Client: dead, Direct: dead,
	}); err == nil {
		t.Fatal("both routes failed but the probe reported success")
	}
}

type rtErr struct{}

func (rtErr) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("proxy: connection reset")
}

// responses_stateful was replaced by responses_mode, and the provider resolved
// both on every request. Folding at load means one field reaches the wire — but
// a nil has to stay nil, or every endpoint without either would be called
// stateful instead of vendor-detected.
func TestLegacyResponsesStatefulFoldsIntoMode(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name     string
		entry    ProviderEntry
		wantMode string
	}{
		{"legacy true becomes stateful", ProviderEntry{ResponsesStateful: &yes}, "stateful"},
		{"legacy false becomes stateless", ProviderEntry{ResponsesStateful: &no}, "stateless"},
		{"the newer field still wins", ProviderEntry{ResponsesMode: "stateless", ResponsesStateful: &yes}, "stateless"},
		{"an unusable mode falls back to the legacy field", ProviderEntry{ResponsesMode: " ", ResponsesStateful: &yes}, "stateful"},
		{"neither set stays undetected", ProviderEntry{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{Providers: []ProviderEntry{c.entry}}
			normalizeLegacyResponsesMode(cfg)
			got := cfg.Providers[0]
			if got.ResponsesMode != c.wantMode {
				t.Fatalf("mode = %q, want %q", got.ResponsesMode, c.wantMode)
			}
			if got.ResponsesStateful != nil {
				t.Fatal("the legacy field survived the fold, so downstream still has two forms to resolve")
			}
		})
	}
}
