// providers.go — adding, listing and removing model providers.
package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
)

// hostGrants are the surfaces a host opens on behalf of its one local client.
// They travel together because they are one decision — "this server is a window,
// not a network service" — and three independent bools spell states no host
// means: signing in but not switching folders, editing providers but not either.
type hostGrants struct {
	workspaceSwitch bool // POST /workspace; see AllowWorkspaceSwitch
	accountAuth     bool // /account routes; see AllowAccountAuth
	providerEdit    bool // /providers writes; see AllowProviderEdit
}

// AllowProviderEdit grants the /providers routes. Off until a host asks:
// adding one writes an API key into the credential store of the machine
// running the kernel, so a server reachable over the network must not let a
// client do it. The desktop shell asks because its only client is its window.
func (s *Server) AllowProviderEdit() { s.grants.providerEdit = true }

func (s *Server) registerProviderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /providers", s.providers)
	mux.HandleFunc("POST /providers", s.saveProvider)
	mux.HandleFunc("POST /providers/probe", s.probeProvider)
	mux.HandleFunc("POST /providers/remove", s.removeProvider)
	mux.HandleFunc("POST /providers/edit", s.editProvider)
	s.registerProviderCheckRoutes(mux)
}

const providerProbeTimeout = 20 * time.Second

// providerNameRE is what may become a config table name and part of a model
// ref ("<provider>/<model>"), so a slash or whitespace cannot be allowed in.
var providerNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// providerView is one configured provider as the panel lists it.
type providerView struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	BaseURL string   `json:"baseUrl"`
	Models  []string `json:"models"`
	// VisionModels is which of them read images, so the form shows the current
	// answer instead of asking the user to remember it.
	VisionModels []string `json:"visionModels"`
	Default      string   `json:"default"`
	HasKey       bool     `json:"hasKey"`
	// KeyEnv names the credential slot. Two entries at one host holding
	// different keys are two accounts and must not be shown as one.
	KeyEnv string `json:"keyEnv,omitempty"`
	// InUse marks the provider the running conversation is on. Removing it
	// would leave the session pointing at a model that no longer exists.
	InUse bool `json:"inUse"`
	// Preset marks an entry that came from the curated catalog rather than
	// from this panel, so the UI can say where it came from.
	Preset bool `json:"preset"`
}

func (s *Server) providers(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	current, _, _ := strings.Cut(currentModelRef(s.ctl()), "/")
	out := make([]providerView, 0, len(cfg.Providers))
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		out = append(out, providerView{
			Name:         p.Name,
			Kind:         strings.ToLower(strings.TrimSpace(p.Kind)),
			BaseURL:      p.BaseURL,
			Models:       nonNilStrings(p.ChatModelList()),
			VisionModels: nonNilStrings(visionModelsOf(cfg, p)),
			Default:      p.DefaultModel(),
			HasKey:       p.APIKey() != "",
			KeyEnv:       p.APIKeyEnv,
			InUse:        p.Name == current,
			Preset:       strings.TrimSpace(p.PresetID) != "",
		})
	}
	writeJSON(w, out)
}

// probeProvider reports what an endpoint turns out to be. It writes nothing:
// the user sees the guesses and confirms them before anything is saved.
func (s *Server) probeProvider(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		http.Error(w, "provider editing is not enabled on this server", http.StatusForbidden)
		return
	}
	var body struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
	}
	if !decodeProviderBody(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), providerProbeTimeout)
	defer cancel()
	proxied, direct := probeClients()
	got, err := config.ProbeEndpoint(ctx, config.ProbeOptions{
		BaseURL: body.BaseURL,
		APIKey:  body.APIKey,
		Client:  proxied,
		Direct:  direct,
	})
	if err != nil {
		// The message is the answer here — "401" and "no chat models" send the
		// user to different fixes — so it is the body, not a bare status.
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, struct {
		Kind       string   `json:"kind"`
		AuthHeader bool     `json:"authHeader"`
		Models     []string `json:"models"`
		Default    string   `json:"default"`
		Efforts    []string `json:"efforts"`
		Effort     string   `json:"effort"`
		Vision     []string `json:"vision"`
		Ambiguous  bool     `json:"ambiguous"`
		NoProxy    bool     `json:"noProxy"`
	}{
		Kind:       got.Kind,
		AuthHeader: got.AuthHeader,
		Models:     nonNilStrings(got.Models),
		Default:    got.Default,
		Efforts:    nonNilStrings(got.Efforts),
		Effort:     got.Effort,
		Vision:     nonNilStrings(got.Vision),
		Ambiguous:  got.Ambiguous,
		NoProxy:    got.NoProxy,
	})
}

// saveProvider writes one provider and its key. The key goes to the credential
// store, never into the config file — a config is something a user pastes into
// an issue.
func (s *Server) saveProvider(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		http.Error(w, "provider editing is not enabled on this server", http.StatusForbidden)
		return
	}
	var body struct {
		Name       string   `json:"name"`
		Kind       string   `json:"kind"`
		BaseURL    string   `json:"baseUrl"`
		APIKey     string   `json:"apiKey"`
		Models     []string `json:"models"`
		Default    string   `json:"default"`
		AuthHeader bool     `json:"authHeader"`
		NoProxy    bool     `json:"noProxy"`
		Effort     string   `json:"effort"`
		Vision     []string `json:"vision"`
	}
	if !decodeProviderBody(w, r, &body) {
		return
	}
	entry, err := providerEntryFrom(body.Name, body.Kind, body.BaseURL, body.Default, body.Effort, body.Models, body.Vision, body.AuthHeader, body.NoProxy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry.APIKeyEnv = keyEnvForNewSource(entry.Name, entry.BaseURL, body.APIKey)
	if key := strings.TrimSpace(body.APIKey); key != "" {
		if _, err := config.SetCredential(entry.APIKeyEnv, key); err != nil {
			http.Error(w, fmt.Sprintf("save provider key: %v", err), http.StatusInternalServerError)
			return
		}
	}
	cfg := config.LoadForEdit(config.UserConfigPath())
	if err := cfg.UpsertProvider(entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// No rebuild: the model list and every switch read config from disk, so the
	// new provider is selectable on the next call without disturbing the
	// conversation that is running.
	writeJSON(w, map[string]any{"name": entry.Name, "models": nonNilStrings(entry.ModelList())})
}

func (s *Server) removeProvider(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		http.Error(w, "provider editing is not enabled on this server", http.StatusForbidden)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeProviderBody(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, "a provider name is required", http.StatusBadRequest)
		return
	}
	if current, _, _ := strings.Cut(currentModelRef(s.ctl()), "/"); current == name {
		// Removing it would leave the conversation on a model that no longer
		// resolves, and the next turn would fail instead of this call.
		http.Error(w, "switch to another model before removing the one in use", http.StatusConflict)
		return
	}
	cfg := config.LoadForEdit(config.UserConfigPath())
	if err := cfg.RemoveProvider(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// providerEntryFrom validates what the panel sent and builds the config entry.
// Only the fields the panel can honestly fill are set; everything else keeps
// its zero value so a hand-edited config is not silently overwritten.
func providerEntryFrom(name, kind, baseURL, def, effort string, models, vision []string, authHeader, noProxy bool) (config.ProviderEntry, error) {
	name = strings.TrimSpace(name)
	if !providerNameRE.MatchString(name) {
		return config.ProviderEntry{}, fmt.Errorf("provider name must be letters, digits, dot, dash or underscore")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "openai" && kind != "anthropic" && kind != "responses" {
		return config.ProviderEntry{}, fmt.Errorf("unsupported provider kind %q", kind)
	}
	if strings.TrimSpace(baseURL) == "" {
		return config.ProviderEntry{}, fmt.Errorf("an endpoint address is required")
	}
	chat := make([]string, 0, len(models))
	for _, m := range models {
		if m = strings.TrimSpace(m); m != "" {
			chat = append(chat, m)
		}
	}
	if len(chat) == 0 {
		return config.ProviderEntry{}, fmt.Errorf("pick at least one model")
	}
	def = strings.TrimSpace(def)
	if def != "" && !slices.Contains(chat, def) {
		return config.ProviderEntry{}, fmt.Errorf("default model %q is not one of the selected models", def)
	}
	return config.ProviderEntry{
		Name:         name,
		Kind:         kind,
		BaseURL:      strings.TrimSpace(baseURL),
		Models:       chat,
		Default:      def,
		APIKeyEnv:    providerKeyEnv(name),
		AuthHeader:   authHeader,
		NoProxy:      noProxy,
		Effort:       strings.TrimSpace(effort),
		VisionModels: vision,
	}, nil
}

// providerKeyEnv is where this provider's key is stored. It is derived from the
// name so two providers never share a slot, and uppercased because that is what
// every other key env in the config looks like.
func providerKeyEnv(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String() + "_API_KEY"
}

// probeClients builds the two routes a probe tries: the user's configured proxy
// first, then a direct one for endpoints that only answer without it.
func probeClients() (proxied, direct *http.Client) {
	spec := netclient.ProxySpec{Mode: netclient.ModeAuto}
	if cfg, err := config.Load(); err == nil && cfg != nil {
		spec = cfg.NetworkProxySpec()
	}
	proxied, _ = netclient.NewHTTPClient(spec, netclient.TransportOptions{})
	direct, _ = netclient.NewHTTPClient(netclient.ProxySpec{Mode: netclient.ModeOff}, netclient.TransportOptions{})
	return proxied, direct
}

func decodeProviderBody(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		http.Error(w, "invalid provider request", http.StatusBadRequest)
		return false
	}
	return true
}

// nonNilStrings keeps an empty list an empty list. A nil slice marshals to
// null, and a client that maps over it takes its whole render down.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
