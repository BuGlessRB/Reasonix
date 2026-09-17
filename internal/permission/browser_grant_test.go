package permission

import (
	"encoding/json"
	"testing"
)

func TestBrowserGrantNamesTheOriginAndCoversEveryBrowserTool(t *testing.T) {
	const github = "https://github.com"
	for _, scope := range []func(string, string) string{SessionGrantRuleForScope, RememberRuleForScope} {
		rule := scope("browser_act", github)
		if rule != "Browser="+github {
			t.Fatalf("grant rule = %q, want Browser=%s", rule, github)
		}
		for _, tool := range []string{"browser_open", "browser_read", "browser_act"} {
			if !SessionGrantMatches(rule, tool, github) {
				t.Errorf("%s on %s is not covered by %q", tool, github, rule)
			}
		}
		if SessionGrantMatches(rule, "browser_open", "https://github.com.evil.example") {
			t.Errorf("%q covered another origin", rule)
		}
		if SessionGrantMatches(rule, "web_fetch", github) {
			t.Errorf("%q covered a tool that is not the browser", rule)
		}
	}
	if SessionGrantMatches("Browser", "browser_act", github) {
		t.Fatal("a bare Browser grant answered for an origin nobody approved")
	}
	if SessionGrantMatches("browser_act", "browser_act", github) {
		t.Fatal("a bare tool-name grant answered for an origin nobody approved")
	}
}

func TestBrowserDecisionsFollowTheModeAndTheOrigin(t *testing.T) {
	args := func(origin string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"steps": []any{}, "origin": origin})
		return raw
	}
	ask := New("ask", nil, nil, []string{"Browser(https://*.bank.example)"})
	if got := ask.Decide("browser_act", false, args("https://example.com")); got != Ask {
		t.Fatalf("ask mode on a new origin = %v, want ask", got)
	}
	if got := ask.Decide("browser_read", true, args("https://example.com")); got != Allow {
		t.Fatalf("reading a page = %v, want allow", got)
	}
	if got := ask.Decide("browser_read", true, args("https://www.bank.example")); got != Deny {
		t.Fatalf("a denied origin = %v, want deny even for a read", got)
	}
	auto := New("allow", nil, nil, nil)
	if got := auto.Decide("browser_act", false, args("https://example.com")); got != Allow {
		t.Fatalf("auto on a new origin = %v, want allow", got)
	}
	if subjectRequiresHuman("browser_act", "https://example.com") {
		t.Fatal("a browser origin must not force a person in auto")
	}
}
