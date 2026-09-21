package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/laya"
)

func TestSystemOneToolReturnsCompleteDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"route":{"type":"choice","choice":"billing","probabilities":{"billing":0.88,"support":0.12},"confidence":0.81}},"usage":{"input_tokens":318,"output_tokens":12}}`))
	}))
	defer server.Close()
	decision := NewSystemOne(SystemOneSpec{HTTP: server.Client(), BaseURL: server.URL, APIKey: func() string { return "secret" }})
	out := runTool(t, decision, map[string]any{
		"state": "payout failed",
		"questions": map[string]any{
			"route": map[string]any{"type": "choice", "instructions": "Choose a team", "criteria": map[string]any{"billing": nil, "support": nil}},
		},
	})
	for _, want := range []string{`"choice":"billing"`, `"billing":0.88`, `"confidence":0.81`, `"input_tokens":318`} {
		if !strings.Contains(out, want) {
			t.Fatalf("result %q does not contain %q", out, want)
		}
	}
}

func TestSystemOneSelectsLayaHTTPBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"english","answers":{"risk":{"type":"noul","noul":0.7}},"routing":{"model":"english"}}`))
	}))
	defer server.Close()
	decision := NewSystemOne(SystemOneSpec{LayaHTTP: &laya.HTTPClient{HTTP: server.Client(), BaseURL: server.URL}})
	out := runTool(t, decision, map[string]any{
		"backend": "laya-http", "state": "cancel my plan",
		"questions": map[string]any{"risk": map[string]any{"type": "noul", "instructions": "Will the user churn?"}},
	})
	if !strings.Contains(out, `"noul":0.7`) || !strings.Contains(out, `"routing"`) {
		t.Fatalf("result = %s", out)
	}
}

func TestSystemOneRequiresBackendWhenSeveralAreConfigured(t *testing.T) {
	decision := NewSystemOne(SystemOneSpec{
		APIKey:   func() string { return "secret" },
		LayaHTTP: &laya.HTTPClient{BaseURL: "https://example.invalid"},
	})
	_, err := decision.Execute(context.Background(), argsJSON(t, map[string]any{
		"state": "x", "questions": map[string]any{"q": map[string]any{"type": "noul", "instructions": "yes?"}},
	}))
	if err == nil || !strings.Contains(err.Error(), "backend is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestSystemOneToolVisibilityFollowsCredential(t *testing.T) {
	key := ""
	decision := NewSystemOne(SystemOneSpec{APIKey: func() string { return key }}).(interface {
		ProviderVisible(context.Context) bool
	})
	if decision.ProviderVisible(context.Background()) {
		t.Fatal("tool visible without a key")
	}
	key = "secret"
	if !decision.ProviderVisible(context.Background()) {
		t.Fatal("tool hidden with a key")
	}
}
