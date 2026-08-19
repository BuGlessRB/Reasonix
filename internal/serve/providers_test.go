package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/history"
	"reasonix/internal/stats"
)

// newProviderEditServer is a server with one configured provider, its config
// and credential store redirected into the test's own home.
func newProviderEditServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	configPath := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `default_model = "existing/model-a"

[[providers]]
name = "existing"
kind = "openai"
base_url = "https://example.invalid/v1"
models = ["model-a"]
default = "model-a"
api_key_env = "EXISTING_API_KEY"
`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Sink: bc, Label: "model-a", ModelRef: "existing/model-a", SessionDir: t.TempDir(),
	})
	t.Cleanup(ctrl.Close)
	closeSharedCatalogsOnCleanup(t)
	return New(ctrl, bc, config.ServeConfig{})
}

// closeSharedCatalogsOnCleanup releases the usage and history catalogs before the
// test's home is removed. They are process-wide, cached per home rather than owned
// by a controller or hub, so no Close reaches them — right in production, wrong
// here: Windows will not unlink an open file, failing a test that already passed.
// Any test pointing REASONIX_HOME at a TempDir needs this.
func closeSharedCatalogsOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = stats.CloseUsageCatalogs(ctx)
		_ = history.CloseSharedCatalog(ctx)
	})
}

func postProvider(t *testing.T, base, path, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The grant is the whole security story: a key lands in the credential store of
// the machine running the kernel, so a reachable server must refuse until its
// host says its only client is a local window.
func TestProviderEditIsRefusedUntilTheHostGrantsIt(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{"/providers", "/providers/probe", "/providers/remove"} {
		resp := postProvider(t, srv.URL, path, `{}`)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("POST %s = %d, want 403 before the host grants provider editing", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	// Listing is not a write and stays readable, so the panel can show what is
	// configured even where adding is refused.
	resp, err := http.Get(srv.URL + "/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /providers = %d, want the listing to stay readable", resp.StatusCode)
	}
}

func TestSaveProviderWritesConfigAndKeepsTheKeyOutOfIt(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers", `{
		"name":"kimi","kind":"openai","baseUrl":"https://api.moonshot.cn/v1",
		"apiKey":"sk-secret-value","models":["kimi-k2","kimi-k2-turbo"],
		"default":"kimi-k2","authHeader":false,"noProxy":false,"effort":"","vision":[]
	}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := readAllString(resp)
		t.Fatalf("POST /providers = %d: %s", resp.StatusCode, b)
	}

	raw, err := os.ReadFile(config.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	written := string(raw)
	if !strings.Contains(written, `name        = "kimi"`) && !strings.Contains(written, `name = "kimi"`) {
		t.Fatalf("provider not written to config:\n%s", written)
	}
	// A config is something a user pastes into an issue. The secret belongs in
	// the credential store, and only its env name belongs here.
	if strings.Contains(written, "sk-secret-value") {
		t.Fatalf("the API key was written into the config file:\n%s", written)
	}
	if !config.CredentialStored("KIMI_API_KEY") {
		t.Fatal("the API key did not reach the credential store")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "kimi" {
			found = true
			if got := cfg.Providers[i].DefaultModel(); got != "kimi-k2" {
				t.Fatalf("default model = %q", got)
			}
		}
	}
	if !found {
		t.Fatal("the saved provider is not visible to a fresh config load")
	}
}

// A whitelist that also raises the provider-wide flag defeats itself: the flag
// answers for every model the list omits, so the text-only half of a mixed
// provider claims image input and the next attached screenshot reaches an
// endpoint that rejects it.
func TestSaveProviderNarrowsVisionToTheListedModels(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers", `{
		"name":"mixed","kind":"openai","baseUrl":"https://gateway.invalid/v1",
		"apiKey":"sk-x","models":["text-only","vision-pro"],
		"default":"text-only","authHeader":false,"noProxy":false,
		"effort":"","vision":["vision-pro"]
	}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := readAllString(resp)
		t.Fatalf("POST /providers = %d: %s", resp.StatusCode, b)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	textOnly, ok := cfg.ResolveModel("mixed/text-only")
	if !ok {
		t.Fatal("ResolveModel did not find mixed/text-only")
	}
	if config.EffectiveVision(textOnly) {
		t.Fatal("a model outside the saved vision list must stay text-only")
	}
	vision, ok := cfg.ResolveModel("mixed/vision-pro")
	if !ok {
		t.Fatal("ResolveModel did not find mixed/vision-pro")
	}
	if !config.EffectiveVision(vision) {
		t.Fatal("a model in the saved vision list must accept images")
	}
}

func TestSaveProviderRejectsWhatItCannotStore(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for name, body := range map[string]string{
		"a name that would break the model ref": `{"name":"a/b","kind":"openai","baseUrl":"https://x.invalid","models":["m"]}`,
		"an unsupported protocol":               `{"name":"x","kind":"grpc","baseUrl":"https://x.invalid","models":["m"]}`,
		"no endpoint":                           `{"name":"x","kind":"openai","baseUrl":"","models":["m"]}`,
		"no models":                             `{"name":"x","kind":"openai","baseUrl":"https://x.invalid","models":[]}`,
		"a default outside the model list":      `{"name":"x","kind":"openai","baseUrl":"https://x.invalid","models":["m"],"default":"other"}`,
	} {
		resp := postProvider(t, srv.URL, "/providers", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// Removing the provider the conversation is on would leave the session pointing
// at a model that no longer resolves, and the next turn — not this call — would
// be the thing that failed.
func TestRemoveProviderRefusesTheOneInUse(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers/remove", `{"name":"existing"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("removing the in-use provider = %d, want 409", resp.StatusCode)
	}
}

func TestRemoveProviderDropsAnUnusedOne(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	add := postProvider(t, srv.URL, "/providers", `{"name":"spare","kind":"openai","baseUrl":"https://x.invalid","models":["m"]}`)
	add.Body.Close()
	resp := postProvider(t, srv.URL, "/providers/remove", `{"name":"spare"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := readAllString(resp)
		t.Fatalf("remove = %d: %s", resp.StatusCode, b)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "spare" {
			t.Fatal("the removed provider is still in the config")
		}
	}
}

func TestProvidersListMarksTheOneInUse(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []providerView
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no providers listed")
	}
	for _, p := range got {
		if p.Name == "existing" && !p.InUse {
			t.Fatalf("the running provider is not marked in use: %+v", p)
		}
		if p.Models == nil {
			t.Fatalf("%s has a nil model list, which marshals to null and breaks the client", p.Name)
		}
	}
}

func TestProviderKeyEnvIsDerivedSafely(t *testing.T) {
	for name, want := range map[string]string{
		"kimi":       "KIMI_API_KEY",
		"my-gateway": "MY_GATEWAY_API_KEY",
		"a.b_c":      "A_B_C_API_KEY",
	} {
		if got := providerKeyEnv(name); got != want {
			t.Errorf("providerKeyEnv(%q) = %q, want %q", name, got, want)
		}
	}
}

func readAllString(resp *http.Response) (string, error) {
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String(), nil
		}
	}
}

// The panel renders its protocol chooser from this, so a wire the kernel can
// build has to reach it without a frontend release.
func TestProviderProtocolsListsEveryWireASourceMayBeSavedAs(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/providers/protocols")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []struct {
		Kind            string `json:"kind"`
		Discovery       string `json:"discovery"`
		ServerWebSearch bool   `json:"serverWebSearch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	byKind := map[string]bool{}
	for _, p := range got {
		byKind[p.Kind] = p.ServerWebSearch
		if p.Kind == "responses" && p.Discovery != "openai" {
			t.Errorf("responses discovered as %q, want the OpenAI model listing", p.Discovery)
		}
	}
	for _, kind := range []string{"openai", "anthropic", "responses"} {
		if _, ok := byKind[kind]; !ok {
			t.Errorf("protocol catalog is missing %q: %#v", kind, got)
		}
	}
	if !byKind["responses"] {
		t.Error("the Responses wire runs web search on the provider and must say so")
	}
}

func TestSaveProviderAcceptsEveryCatalogedWire(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers", `{
		"name":"deepseek-responses","kind":"responses","baseUrl":"https://api.deepseek.com",
		"apiKey":"sk-value","models":["deepseek-v4-flash"],"default":"deepseek-v4-flash",
		"authHeader":false,"noProxy":false,"effort":"","vision":[]
	}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := readAllString(resp)
		t.Fatalf("POST /providers with kind=responses = %d: %s", resp.StatusCode, b)
	}
	raw, err := os.ReadFile(config.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"responses"`) {
		t.Fatalf("the Responses source did not reach the config:\n%s", raw)
	}
}
