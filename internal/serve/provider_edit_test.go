package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

// A source the panel cannot fully describe: per-model prices and effort
// vocabularies, a context window, a wallet endpoint. Editing its model list must
// not cost the user any of it.
const richProviderConfig = `default_model = "rich/alpha"

[[providers]]
name = "rich"
kind = "openai"
base_url = "https://gateway.invalid/v1"
models = ["alpha", "beta"]
default = "alpha"
api_key_env = "RICH_API_KEY"
balance_url = "https://gateway.invalid/balance"
context_window = 131072
thinking = "enabled"

[providers.prices.alpha]
input = 1.0
output = 2.0
currency = "CNY"

[providers.model_overrides.beta]
supported_efforts = ["low", "high"]
default_effort = "high"
`

func newRichProviderServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	if _, err := config.SetCredential("RICH_API_KEY", "sk-rich"); err != nil {
		t.Fatal(err)
	}
	path := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(richProviderConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Sink: bc, Label: "alpha", ModelRef: "rich/alpha", SessionDir: t.TempDir(),
	})
	s := New(ctrl, bc, config.ServeConfig{})
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func TestEditProviderKeepsWhatTheFormCannotShow(t *testing.T) {
	srv := newRichProviderServer(t)

	resp := postProvider(t, srv.URL, "/providers/edit", `{
		"name":"rich","baseUrl":"https://gateway.invalid/v1",
		"models":["alpha","beta"],"default":"beta","vision":["beta"]
	}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := readAllString(resp)
		t.Fatalf("POST /providers/edit = %d: %s", resp.StatusCode, b)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.Provider("rich")
	if !ok {
		t.Fatal("the provider disappeared")
	}
	if entry.ContextWindow != 131072 || entry.Thinking != "enabled" || entry.BalanceURL == "" {
		t.Fatalf("the edit flattened provider-wide fields: %+v", entry)
	}
	if entry.Prices["alpha"] == nil || entry.Prices["alpha"].Input != 1 {
		t.Fatalf("the per-model price did not survive: %+v", entry.Prices)
	}
	if got := entry.ModelOverrides["beta"].SupportedEfforts; len(got) != 2 {
		t.Fatalf("the per-model effort list did not survive: %v", got)
	}
	if entry.Default != "beta" {
		t.Fatalf("default = %q, want the edited beta", entry.Default)
	}
}

// The toggle has to be the whole answer. A provider-wide flag answers for every
// model the whitelist omits, so writing only the list would leave the switch
// looking broken.
func TestEditProviderVisionSelectionIsAuthoritative(t *testing.T) {
	srv := newRichProviderServer(t)

	resp := postProvider(t, srv.URL, "/providers/edit", `{
		"name":"rich","models":["alpha","beta"],"default":"alpha","vision":["beta"]
	}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /providers/edit = %d", resp.StatusCode)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	beta, ok := cfg.ResolveModel("rich/beta")
	if !ok || !config.EffectiveVision(beta) {
		t.Fatal("the model the user ticked does not read images")
	}
	alpha, ok := cfg.ResolveModel("rich/alpha")
	if !ok || config.EffectiveVision(alpha) {
		t.Fatal("a model the user left unticked still claims image input")
	}
}

// The list has to show the current answer, or the form asks the user to
// remember which models read images.
func TestProvidersListReportsWhichModelsReadImages(t *testing.T) {
	srv := newRichProviderServer(t)
	postProvider(t, srv.URL, "/providers/edit", `{
		"name":"rich","models":["alpha","beta"],"default":"alpha","vision":["beta"]
	}`).Body.Close()

	resp, err := http.Get(srv.URL + "/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []providerView
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.Name != "rich" {
			continue
		}
		if len(p.VisionModels) != 1 || p.VisionModels[0] != "beta" {
			t.Fatalf("visionModels = %v, want just beta", p.VisionModels)
		}
		return
	}
	t.Fatal("the provider is missing from the list")
}

func TestEditProviderRefusesWhatItCannotApply(t *testing.T) {
	srv := newRichProviderServer(t)

	for name, body := range map[string]string{
		"a provider that is gone":          `{"name":"nobody","models":["alpha"]}`,
		"no models":                        `{"name":"rich","models":[]}`,
		"a default outside the model list": `{"name":"rich","models":["alpha"],"default":"beta"}`,
	} {
		resp := postProvider(t, srv.URL, "/providers/edit", body)
		if resp.StatusCode == http.StatusNoContent {
			t.Errorf("%s: the edit was accepted", name)
		}
		resp.Body.Close()
	}
}
