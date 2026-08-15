// provider_edit.go — changing a source without flattening what it already knows.
package serve

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"reasonix/internal/config"
)

// editProvider changes only the fields this panel owns and leaves the rest of
// the entry alone. UpsertProvider replaces an entry wholesale, so building one
// from the form would drop every field the form cannot show — per-model prices,
// effort vocabularies, context windows, headers, the preset it came from.
func (s *Server) editProvider(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		http.Error(w, "provider editing is not enabled on this server", http.StatusForbidden)
		return
	}
	var body struct {
		Name    string   `json:"name"`
		BaseURL string   `json:"baseUrl"`
		APIKey  string   `json:"apiKey"`
		Models  []string `json:"models"`
		Default string   `json:"default"`
		Vision  []string `json:"vision"`
	}
	if !decodeProviderBody(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	edit := config.LoadForEdit(config.UserConfigPath())
	entry, ok := edit.Provider(name)
	if !ok {
		http.Error(w, fmt.Sprintf("no provider named %q", name), http.StatusNotFound)
		return
	}
	models := trimmedNonEmpty(body.Models)
	if len(models) == 0 {
		http.Error(w, "pick at least one model", http.StatusBadRequest)
		return
	}
	def := strings.TrimSpace(body.Default)
	if def != "" && !slices.Contains(models, def) {
		http.Error(w, fmt.Sprintf("default model %q is not one of the selected models", def), http.StatusBadRequest)
		return
	}
	if base := strings.TrimSpace(body.BaseURL); base != "" {
		entry.BaseURL = base
	}
	entry.Models = models
	entry.Model = ""
	entry.Default = def
	applyVisionSelection(entry, trimmedNonEmpty(body.Vision))

	if key := strings.TrimSpace(body.APIKey); key != "" {
		if _, err := config.SetCredential(entry.APIKeyEnv, key); err != nil {
			http.Error(w, fmt.Sprintf("save provider key: %v", err), http.StatusInternalServerError)
			return
		}
	}
	if err := edit.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyVisionSelection makes the panel's list the whole answer. The provider-wide
// flag answers for every model a whitelist omits, and a per-model override beats
// both, so a toggle that only wrote the list would leave the user flipping a
// switch that changes nothing.
func applyVisionSelection(entry *config.ProviderEntry, vision []string) {
	entry.VisionModels = vision
	entry.Vision = false
	for _, model := range entry.Models {
		override, ok := entry.ModelOverrides[model]
		if !ok || override.Vision == nil {
			continue
		}
		reads := slices.Contains(vision, model)
		override.Vision = &reads
		entry.ModelOverrides[model] = override
	}
}

func trimmedNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// visionModelsOf answers per model rather than reading the whitelist, so a
// provider-wide flag or a per-model override shows up as the checkbox it is.
func visionModelsOf(cfg *config.Config, p *config.ProviderEntry) []string {
	out := make([]string, 0, len(p.Models))
	for _, model := range p.ChatModelList() {
		if entry, ok := cfg.ResolveModel(p.Name + "/" + model); ok && config.EffectiveVision(entry) {
			out = append(out, model)
		}
	}
	return out
}

// setProviderWebSearch records the tri-state for the endpoint-executed search
// tool. It is a real per-entry choice, unlike the protocol: the wire format is
// what makes it available, and this only says whether to use it.
func (s *Server) setProviderWebSearch(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		http.Error(w, "provider editing is not enabled on this server", http.StatusForbidden)
		return
	}
	var body struct {
		Name string `json:"name"`
		On   bool   `json:"on"`
	}
	if !decodeProviderBody(w, r, &body) {
		return
	}
	edit := config.LoadForEdit(config.UserConfigPath())
	entry, ok := edit.Provider(strings.TrimSpace(body.Name))
	if !ok {
		http.Error(w, fmt.Sprintf("no provider named %q", body.Name), http.StatusNotFound)
		return
	}
	if !config.SupportsServerWebSearch(entry) {
		http.Error(w, "this protocol has no wire format for a provider-executed web search", http.StatusBadRequest)
		return
	}
	on := body.On
	entry.WebSearch = &on
	if err := edit.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
