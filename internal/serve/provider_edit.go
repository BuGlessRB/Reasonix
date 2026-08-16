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
		refuse(w, http.StatusForbidden, "provider.editing_disabled", "provider editing is not enabled on this server", nil)
		return
	}
	// The three compatibility fields are pointers so that "not sent" and "sent
	// empty" stay different answers: a client that does not show them must not
	// silently clear the headers a gateway needs.
	var body struct {
		Name          string             `json:"name"`
		BaseURL       string             `json:"baseUrl"`
		APIKey        string             `json:"apiKey"`
		Models        []string           `json:"models"`
		Default       string             `json:"default"`
		Vision        []string           `json:"vision"`
		ContextWindow *int               `json:"contextWindow"`
		Headers       *map[string]string `json:"headers"`
		ExtraBody     *map[string]any    `json:"extraBody"`
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
		refuse(w, http.StatusBadRequest, "provider.no_models_picked", "pick at least one model", nil)
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

	if body.ContextWindow != nil {
		// Zero is a real answer here — it turns automatic compaction off for
		// this source — so only a negative one is a mistake.
		if *body.ContextWindow < 0 {
			refuse(w, http.StatusBadRequest, "provider.bad_context_window", "context window cannot be negative", nil)
			return
		}
		entry.ContextWindow = *body.ContextWindow
	}
	if body.Headers != nil {
		entry.Headers = trimmedHeaders(*body.Headers)
	}
	if body.ExtraBody != nil {
		// A null cannot be written to TOML, so it would be dropped on save and
		// the field would silently never reach the wire.
		if path, ok := firstNullPath(*body.ExtraBody, ""); ok {
			refuse(w, http.StatusBadRequest, "provider.extra_body_null",
				fmt.Sprintf("extra body field %q cannot be null", path), map[string]any{"path": path})
			return
		}
		entry.ExtraBody = *body.ExtraBody
	}

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

// trimmedHeaders drops blank names and values: a header with an empty name is
// not a header, and one with an empty value is a line the user was still typing.
func trimmedHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// firstNullPath reports the first null anywhere in the object, named by its
// path so the message points at the line to fix rather than at the whole field.
func firstNullPath(v any, path string) (string, bool) {
	switch t := v.(type) {
	case nil:
		return path, true
	case map[string]any:
		for k, inner := range t {
			at := k
			if path != "" {
				at = path + "." + k
			}
			if found, ok := firstNullPath(inner, at); ok {
				return found, true
			}
		}
	case []any:
		for i, inner := range t {
			if found, ok := firstNullPath(inner, fmt.Sprintf("%s[%d]", path, i)); ok {
				return found, true
			}
		}
	}
	return "", false
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

// setProviderThinking pins or releases the plain-chat request shape. Relays
// that reject an unknown thinking/reasoning_effort field fail every request
// until this is off, and the endpoint's own error rarely names the field.
func (s *Server) setProviderThinking(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		refuse(w, http.StatusForbidden, "provider.editing_disabled", "provider editing is not enabled on this server", nil)
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
	if !config.CanConfigureThinkingParams(entry) {
		refuse(w, http.StatusBadRequest, "provider.no_thinking_param", "this protocol never sends thinking parameters", nil)
		return
	}
	config.SetThinkingParams(entry, body.On)
	if err := edit.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setProviderWebSearch records the tri-state for the endpoint-executed search
// tool. It is a real per-entry choice, unlike the protocol: the wire format is
// what makes it available, and this only says whether to use it.
func (s *Server) setProviderWebSearch(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		refuse(w, http.StatusForbidden, "provider.editing_disabled", "provider editing is not enabled on this server", nil)
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
		refuse(w, http.StatusBadRequest, "provider.no_websearch_wire", "this protocol has no wire format for a provider-executed web search", nil)
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
