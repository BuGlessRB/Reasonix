// provider_check.go — asking a configured endpoint what it still is.
package serve

import (
	"context"
	"net/http"
	"strings"

	"reasonix/internal/config"
)

// providerCheck is what re-probing a saved provider found. Kind is the protocol
// the endpoint answered to, which is not always the one the entry claims: a
// gateway that changed hands, or a guess made when both protocols replied.
type providerCheck struct {
	OK   bool   `json:"ok"`
	Kind string `json:"kind,omitempty"`
	// Matches is whether that answer is consistent with the kind the entry
	// declares. Protocols sharing a listing shape are consistent with each
	// other, so a Responses source answering the OpenAI listing is not a change.
	Matches   bool     `json:"matches"`
	Models    []string `json:"models,omitempty"`
	Ambiguous bool     `json:"ambiguous,omitempty"`
	NoProxy   bool     `json:"noProxy,omitempty"`
	// Error carries the endpoint's own words. "401" and "no chat models" send
	// the user to different fixes, so the message is the answer here.
	Error string `json:"error,omitempty"`
}

// No protocol switch rides along with this. A probe only lists models, and it
// tries both auth shapes against the same listing URLs, so "both answered" says
// nothing about whether both chat contracts live at this base_url — DeepSeek
// serves OpenAI chat at /chat/completions and Anthropic at /anthropic/v1/messages.
// Flipping kind alone would aim one contract at the other's address.
func (s *Server) registerProviderCheckRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /providers/check", s.checkProvider)
}

// checkProvider re-runs the add-a-source probe against what is already saved, so
// "is this key still good, and is it still the protocol we recorded" is one
// button rather than a delete and a re-add.
func (s *Server) checkProvider(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		refuse(w, http.StatusForbidden, "provider.editing_disabled", "provider editing is not enabled on this server", nil)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeProviderBody(w, r, &body) {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	entry, ok := cfg.Provider(strings.TrimSpace(body.Name))
	if !ok {
		notFound(w, "provider", body.Name)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), providerProbeTimeout)
	defer cancel()
	proxied, direct := probeClients()
	got, probeErr := config.ProbeEndpoint(ctx, config.ProbeOptions{
		BaseURL: entry.BaseURL,
		APIKey:  entry.APIKey(),
		Client:  proxied,
		Direct:  direct,
	})
	// A refusal is a finding, not a request failure: the row wants to say what
	// went wrong, and a bare status code would leave it with nothing to show.
	if probeErr != nil {
		writeJSON(w, providerCheck{Error: probeErr.Error()})
		return
	}
	writeJSON(w, providerCheck{
		OK:        true,
		Kind:      got.Kind,
		Matches:   config.ProtocolAnswerMatches(entry.Kind, got.Kind),
		Models:    nonNilStrings(got.Models),
		Ambiguous: got.Ambiguous,
		NoProxy:   got.NoProxy,
	})
}
