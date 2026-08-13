package serve

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"reasonix/internal/agentpreset"
	"reasonix/internal/config"
)

func (s *Server) preset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Preset string `json:"preset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !agentpreset.IsValid(body.Preset) {
		http.Error(w, "unknown preset", http.StatusBadRequest)
		return
	}
	s.ctl().SetAgentPreset(body.Preset)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) model(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Ref) == "" {
		http.Error(w, "missing ref", http.StatusBadRequest)
		return
	}
	if err := s.switchModel(r.Context(), strings.TrimSpace(body.Ref)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) effort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Effort string `json:"effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Effort) == "" {
		http.Error(w, "missing effort", http.StatusBadRequest)
		return
	}
	if err := s.switchEffort(r.Context(), strings.TrimSpace(body.Effort)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// status returns a combined status snapshot.
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	used, window := s.ctl().ContextSnapshot()
	hit, miss := s.ctl().SessionCache()
	sess := map[string]any{
		"label":            s.ctl().Label(),
		"running":          s.ctl().Running(),
		"plan":             s.ctl().PlanMode(),
		"autoApproveTools": s.ctl().AutoApproveTools(),
		"bypass":           s.ctl().AutoApproveTools(),
		"toolApprovalMode": s.ctl().ToolApprovalMode(),
		"preset":           s.ctl().AgentPreset(),
		"goal":             s.ctl().Goal(),
		"goalStatus":       s.ctl().GoalStatus(),
		"cwd":              s.ctl().SessionDir(),
		"workspaceRoot":    s.ctl().WorkspaceRoot(),
		"sessionPath":      s.ctl().SessionPath(),
		"used":             used,
		"window":           window,
		"cacheHit":         hit,
		"cacheMiss":        miss,
	}
	if u := s.ctl().LastUsage(); u != nil {
		sess["lastUsage"] = u
	}
	if b, err := s.ctl().Balance(r.Context()); err == nil && b != nil {
		if cfg, loadErr := config.Load(); loadErr == nil && cfg.DisplayCurrencyPref() == "" {
			// Runtime-only hint: a single wallet currency may select an existing
			// valuation, but is never persisted as configuration or history.
			s.bc.SetDisplayCurrency(b.PrimaryCurrency())
		}
		sess["balance"] = map[string]any{
			"display":   b.Display(),
			"available": b.Available,
			"infos":     b.Infos,
		}
	} else if err != nil {
		slog.Warn("serve: balance fetch failed", "err", err)
	}
	if cfg, err := config.Load(); err == nil {
		if entry, ok := cfg.ResolveModel(currentModelRef(s.ctl())); ok {
			sess["effort"] = entry.Effort
			sess["modelRef"] = entry.Name + "/" + entry.Model
		}
	}
	sess["sessionCostQuote"] = s.bc.SessionCostQuote()
	if j := s.ctl().Jobs(); len(j) > 0 {
		sess["jobs"] = j
	}
	writeJSON(w, sess)
}
