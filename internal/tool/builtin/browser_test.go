package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/browser"
)

func TestBrowserPermissionArgsCarryTheHostsOrigin(t *testing.T) {
	session := browser.NewSession(browser.Config{})
	open := browserOpen{session: session}
	got := open.PermissionArgs(context.Background(), json.RawMessage(`{"url":"HTTPS://GitHub.com:8443/a?b","origin":"https://trusted.example"}`))
	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["origin"] != "https://github.com:8443" || fields["url"] == nil {
		t.Fatalf("browser_open permission args = %s", got)
	}
	act := browserAct{session: session}
	got = act.PermissionArgs(context.Background(), json.RawMessage(`{"steps":[],"origin":"https://trusted.example"}`))
	fields = nil
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["origin"]; ok {
		t.Fatalf("an act with no page open kept the model's origin: %s", got)
	}
	if got := (browserOpen{}).PermissionArgs(context.Background(), json.RawMessage(`{"url":"file:///w/index.html"}`)); !json.Valid(got) || string(got) != `{"origin":"file://","url":"file:///w/index.html"}` {
		t.Fatalf("file url permission args = %s", got)
	}
}

func TestUnboundBrowserToolsSayThereIsNoBrowser(t *testing.T) {
	for _, tl := range []interface {
		Execute(context.Context, json.RawMessage) (string, error)
	}{browserOpen{}, browserRead{}, browserAct{}} {
		_, err := tl.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com","steps":[{"action":"click"}]}`))
		if browser.CodeOf(err) != browser.CodeEngineMissing {
			t.Fatalf("%T without a browser = %v", tl, err)
		}
	}
	if BrowserBound(browserOpen{}) || !BrowserBound(browserAct{session: browser.NewSession(browser.Config{})}) {
		t.Fatal("BrowserBound misreports")
	}
}
