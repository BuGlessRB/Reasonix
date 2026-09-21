package serve

import (
	"encoding/json"
	"net/http"
	"strings"

	"reasonix/internal/config"
)

type decisionModelsView struct {
	TypeSafe struct {
		BaseURL string `json:"baseUrl"`
		Model   string `json:"model"`
		KeyEnv  string `json:"keyEnv"`
		HasKey  bool   `json:"hasKey"`
		APIKey  string `json:"apiKey,omitempty"`
	} `json:"typeSafe"`
	Laya struct {
		Local       bool   `json:"local"`
		Python      string `json:"python"`
		Model       string `json:"model"`
		HTTPBaseURL string `json:"httpBaseUrl"`
		HTTPKeyEnv  string `json:"httpKeyEnv"`
		HasHTTPKey  bool   `json:"hasHttpKey"`
		HTTPAPIKey  string `json:"httpApiKey,omitempty"`
	} `json:"laya"`
}

func (s *Server) registerDecisionModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /decision-models", s.decisionModels)
	mux.HandleFunc("POST /decision-models", s.saveDecisionModels)
}

func (s *Server) decisionModels(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, decisionModelsFrom(cfg))
}

func decisionModelsFrom(cfg *config.Config) decisionModelsView {
	var out decisionModelsView
	c := cfg.Tools.SystemOne
	out.TypeSafe.BaseURL, out.TypeSafe.Model, out.TypeSafe.KeyEnv = c.BaseURL, c.Model, c.APIKeyEnv
	out.TypeSafe.HasKey = cfg.SystemOneAPIKey() != ""
	out.Laya.Local, out.Laya.Python, out.Laya.Model = c.Laya.Local, c.Laya.Python, c.Laya.Model
	out.Laya.HTTPBaseURL, out.Laya.HTTPKeyEnv = c.Laya.HTTPBaseURL, c.Laya.HTTPAPIKeyEnv
	out.Laya.HasHTTPKey = cfg.LayaHTTPAPIKey() != ""
	return out
}

func (s *Server) saveDecisionModels(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		refuse(w, http.StatusForbidden, "provider.editing_disabled", "provider editing is not enabled on this server", nil)
		return
	}
	var body decisionModelsView
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	edit := config.LoadForEdit(config.UserConfigPath())
	edit.Tools.SystemOne.BaseURL = strings.TrimSpace(body.TypeSafe.BaseURL)
	edit.Tools.SystemOne.Model = strings.TrimSpace(body.TypeSafe.Model)
	edit.Tools.SystemOne.APIKeyEnv = strings.TrimSpace(body.TypeSafe.KeyEnv)
	if edit.Tools.SystemOne.APIKeyEnv == "" {
		edit.Tools.SystemOne.APIKeyEnv = "TYPESAFE_API_KEY"
	}
	edit.Tools.SystemOne.Laya.Local = body.Laya.Local
	edit.Tools.SystemOne.Laya.Python = strings.TrimSpace(body.Laya.Python)
	edit.Tools.SystemOne.Laya.Model = strings.TrimSpace(body.Laya.Model)
	edit.Tools.SystemOne.Laya.HTTPBaseURL = strings.TrimSpace(body.Laya.HTTPBaseURL)
	edit.Tools.SystemOne.Laya.HTTPAPIKeyEnv = strings.TrimSpace(body.Laya.HTTPKeyEnv)
	if key := strings.TrimSpace(body.TypeSafe.APIKey); key != "" {
		if _, err := config.SetCredential(edit.Tools.SystemOne.APIKeyEnv, key); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	if key := strings.TrimSpace(body.Laya.HTTPAPIKey); key != "" {
		if edit.Tools.SystemOne.Laya.HTTPAPIKeyEnv == "" {
			edit.Tools.SystemOne.Laya.HTTPAPIKeyEnv = "LAYA_API_KEY"
		}
		if _, err := config.SetCredential(edit.Tools.SystemOne.Laya.HTTPAPIKeyEnv, key); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := edit.SaveTo(config.UserConfigPath()); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.rebuildInPlace(r.Context()); err != nil {
		rebuildFailed(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
