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

type modelEntry struct {
	Ref      string `json:"ref"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Kind     string `json:"kind,omitempty"`
	Active   bool   `json:"active,omitempty"`
	Default  bool   `json:"default,omitempty"`
}

type modelRoute struct {
	key  string
	solo bool
}

// collapseModelRoutes drops entries naming the same model at the same endpoint.
// A multi-model provider block and a single-model block pinning one of them (to
// attach its own price) are two config rows, not two models. The survivor is the
// active one, so the current selection stays selectable, then the default, then
// the single-model block, whose price table is the exact one.
func collapseModelRoutes(entries []modelEntry, routes []modelRoute) []modelEntry {
	if len(routes) == 0 {
		return entries
	}
	best := make(map[string]int, len(routes))
	for i := range routes {
		j, seen := best[routes[i].key]
		if !seen || betterModelRoute(entries[i], routes[i], entries[j], routes[j]) {
			best[routes[i].key] = i
		}
	}
	kept := entries[:0:0]
	for i := range entries {
		if i < len(routes) && best[routes[i].key] != i {
			continue
		}
		kept = append(kept, entries[i])
	}
	return kept
}

func betterModelRoute(a modelEntry, ar modelRoute, b modelEntry, br modelRoute) bool {
	if a.Active != b.Active {
		return a.Active
	}
	if a.Default != b.Default {
		return a.Default
	}
	return ar.solo && !br.solo
}
