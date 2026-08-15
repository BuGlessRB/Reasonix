package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/config"
)

func savedProvider(t *testing.T, name string) *config.ProviderEntry {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.Provider(name)
	if !ok {
		t.Fatalf("provider %q was not saved", name)
	}
	return entry
}

// Adding the same endpoint under the other protocol is another door onto one
// account. Without a shared credential slot the second entry has no key, and
// the pair reads as two accounts in a list that groups on host and key.
func TestAddingASecondDoorReusesTheHostsKey(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	first := postProvider(t, srv.URL, "/providers", `{
		"name":"acme","kind":"openai","baseUrl":"https://api.acme.test/v1",
		"apiKey":"sk-acme","models":["m1"],"default":"m1"
	}`)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first save = %d", first.StatusCode)
	}

	// No key: the user is adding the other protocol for the account already here.
	second := postProvider(t, srv.URL, "/providers", `{
		"name":"acme-anthropic","kind":"anthropic","baseUrl":"https://api.acme.test/anthropic",
		"apiKey":"","models":["m1"],"default":"m1"
	}`)
	second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("second save = %d", second.StatusCode)
	}

	one, two := savedProvider(t, "acme"), savedProvider(t, "acme-anthropic")
	if one.APIKeyEnv != two.APIKeyEnv {
		t.Fatalf("key slots differ: %q vs %q — the doors would read as two accounts", one.APIKeyEnv, two.APIKeyEnv)
	}
	if two.APIKey() == "" {
		t.Fatal("the second door has no credential, so nothing it lists is selectable")
	}
}

// A key of its own means a different account — two tenants of one relay. Reusing
// the slot there would overwrite the first tenant's credential.
func TestAddingASecondAccountAtOneHostKeepsItsOwnKey(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	postProvider(t, srv.URL, "/providers", `{
		"name":"relay","kind":"openai","baseUrl":"https://relay.test/v1",
		"apiKey":"sk-one","models":["m1"],"default":"m1"
	}`).Body.Close()
	postProvider(t, srv.URL, "/providers", `{
		"name":"relay-work","kind":"openai","baseUrl":"https://relay.test/v1",
		"apiKey":"sk-two","models":["m1"],"default":"m1"
	}`).Body.Close()

	one, two := savedProvider(t, "relay"), savedProvider(t, "relay-work")
	if one.APIKeyEnv == two.APIKeyEnv {
		t.Fatalf("both tenants share slot %q, so saving the second overwrote the first", one.APIKeyEnv)
	}
	if one.APIKey() != "sk-one" || two.APIKey() != "sk-two" {
		t.Fatalf("credentials crossed: %q and %q", one.APIKey(), two.APIKey())
	}
}

// A first source at an unseen host has nothing to inherit.
func TestFirstSourceAtAHostGetsItsOwnKeySlot(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	postProvider(t, srv.URL, "/providers", `{
		"name":"fresh","kind":"openai","baseUrl":"https://fresh.test/v1",
		"apiKey":"sk-fresh","models":["m1"],"default":"m1"
	}`).Body.Close()

	if got := savedProvider(t, "fresh").APIKeyEnv; got != "FRESH_API_KEY" {
		t.Fatalf("key slot = %q, want the one derived from its own name", got)
	}
}
