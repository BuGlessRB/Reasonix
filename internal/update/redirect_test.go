package update

import (
	"errors"
	"net/http"
	"testing"
)

func redirectTo(t *testing.T, raw string, hops int) error {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		t.Fatalf("NewRequest(%q): %v", raw, err)
	}
	return TrustedRedirect(req, make([]*http.Request, hops))
}

// Every field this guard reads is one an attacker controls once a release host
// answers with a redirect, so each is stated as its own case rather than as one
// "bad URL" — a guard that stopped checking the scheme would still pass a test
// that only ever handed it another hostname.
func TestTrustedRedirectRefusesWhatItWasWrittenFor(t *testing.T) {
	for _, tc := range []struct{ name, url string }{
		{"a plain-HTTP downgrade", "http://dl.reasonix.io/studio/x.tar.gz"},
		{"credentials smuggled in the authority", "https://user:pw@dl.reasonix.io/x"},
		{"a port, which no release host answers on", "https://dl.reasonix.io:8443/x"},
		{"a host that only looks like ours", "https://dl.reasonix.io.evil.test/x"},
		{"an unrelated host outright", "https://evil.test/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := redirectTo(t, tc.url, 1); err == nil {
				t.Fatalf("TrustedRedirect(%q) = nil, want a refusal", tc.url)
			}
		})
	}
	if err := redirectTo(t, "https://dl.reasonix.io/x", 10); err == nil {
		t.Fatal("a chain of ten hops was followed; want it stopped")
	}
}

// The hosts releases are actually served from. A guard that refused these would
// break every update rather than protect one.
func TestTrustedRedirectPassesTheReleaseHosts(t *testing.T) {
	for _, raw := range []string{
		"https://dl.reasonix.io/studio/versions.json",
		"https://reasonix.io/studio/x",
		"https://github.com/esengine/DeepSeek-Reasonix/releases/download/v1/x",
		"https://objects.githubusercontent.com/blob/x",
	} {
		if err := redirectTo(t, raw, 1); err != nil {
			t.Fatalf("TrustedRedirect(%q) = %v, want it followed", raw, err)
		}
	}
}

// The guard sat unreferenced while the downloads it was written for followed
// redirects wherever they pointed: the shells assemble their client from
// netclient, which sets no policy at all. New is where that is closed, and it
// copies because that client is shared with the rest of the app.
func TestNewGuardsTheClientsItWasHanded(t *testing.T) {
	caller := &http.Client{}
	fallback := &http.Client{}
	u := New(Options{HTTP: caller, Fallback: fallback})

	if u.opts.HTTP.CheckRedirect == nil || u.opts.Fallback.CheckRedirect == nil {
		t.Fatal("an updater fetched release bytes with no redirect policy")
	}
	if caller.CheckRedirect != nil || fallback.CheckRedirect != nil {
		t.Fatal("the caller's own client was mutated; it is shared with the rest of the app")
	}
	if u.opts.HTTP == caller || u.opts.Fallback == fallback {
		t.Fatal("the guarded client is the caller's, not a copy of it")
	}
}

func TestNewKeepsAPolicyTheCallerAlreadyChose(t *testing.T) {
	chosen := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u := New(Options{HTTP: &http.Client{CheckRedirect: chosen}})
	if err := u.opts.HTTP.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatal("New replaced a redirect policy its caller had already decided")
	}
}

// A nil client is how the version panel is built before a proxy is resolved;
// guarding must not turn that into a client that exists.
func TestNewLeavesAnAbsentClientAbsent(t *testing.T) {
	if u := New(Options{}); u.opts.HTTP != nil || u.opts.Fallback != nil {
		t.Fatal("guarding invented a client out of nil")
	}
}
