package serve

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"reasonix/internal/agentpreset"
	"reasonix/internal/config"
	"reasonix/internal/provider"
)

func (s *Server) preset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Preset string `json:"preset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badBody(w)
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
		missingField(w, "ref")
		return
	}
	ref := strings.TrimSpace(body.Ref)
	if err := s.switchModel(r.Context(), ref); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	catalog := s.ctl().ProviderCatalog()
	// The switch only rebuilt the running controller. Without this the next
	// launch boots from default_model and lands back on whatever was there
	// before, which reads as the choice not having been saved at all — the CLI
	// and the old desktop have both persisted it since they had a picker.
	persistDefaultModel(ref, catalog)
	w.WriteHeader(http.StatusNoContent)
}

// persistDefaultModel records the choice in the user config. A refusal is worth
// a log and nothing more: the live switch already succeeded, and failing the
// request would say the model did not change when it did.
func persistDefaultModel(ref string, catalog []provider.Descriptor) {
	path := config.UserConfigPath()
	if path == "" {
		return
	}
	// Serialize against other in-process editors so concurrent writers do not
	// drop each other's fields.
	unlock := config.LockUserConfigEdits()
	defer unlock()
	edit := config.LoadForEdit(path)
	if err := edit.SetDefaultModel(ref, catalog); err != nil {
		slog.Warn("serve: persist default model", "ref", ref, "err", err)
		return
	}
	if err := edit.SaveTo(path); err != nil {
		slog.Warn("serve: save default model", "ref", ref, "path", path, "err", err)
	}
}

// applyApprovalMode switches the live posture and records it. The two belong
// together: a switch that only lands at runtime reads, one launch later, as a
// choice that was never made.
func (s *Server) applyApprovalMode(mode string) {
	s.ctl().SetToolApprovalMode(mode)
	persistDesktopApprovalMode(mode)
}

// persistDesktopApprovalMode records the posture chosen on the composer. Same
// reasoning as the model above: without it the next launch reads as the choice
// never having been made. The refusal path is a log, not a failed request — the
// live switch already took effect.
func persistDesktopApprovalMode(mode string) {
	path := config.UserConfigPath()
	if path == "" {
		return
	}
	unlock := config.LockUserConfigEdits()
	defer unlock()
	edit := config.LoadForEdit(path)
	if err := edit.SetDesktopDefaultToolApprovalMode(mode); err != nil {
		slog.Warn("serve: persist approval mode", "mode", mode, "err", err)
		return
	}
	if err := edit.SaveTo(path); err != nil {
		slog.Warn("serve: save approval mode", "mode", mode, "path", path, "err", err)
	}
}

func (s *Server) effort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Effort string `json:"effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Effort) == "" {
		missingField(w, "effort")
		return
	}
	if err := s.switchEffort(r.Context(), strings.TrimSpace(body.Effort)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
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
			// Whether this model reads images at all. A composer that cannot ask
			// lets the user paste a screenshot into a text-only model and watch
			// nothing happen.
			sess["vision"] = config.EffectiveVision(entry)
			// And whether that false is an answer or a silence: a relay forwards
			// models nothing here has a label for, and telling its user the model
			// cannot read images states a limitation that was never established.
			sess["visionDeclared"] = config.VisionDeclared(entry)
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
	// Vendor is the endpoint host. Entries sharing it are one service reached
	// under different protocols, which is what lets a picker fold the routes.
	Vendor string `json:"vendor,omitempty"`
	// KeyEnv is the credential this route spends. It pairs with Vendor to
	// identify the account: one host can hold more than one.
	KeyEnv string `json:"keyEnv,omitempty"`
	// Preset marks a name we shipped rather than one the user chose, so a picker
	// knows whose name Provider is before putting it on screen.
	Preset bool `json:"preset,omitempty"`
	// The capability face; see describeModel. Omitted fields mean "nothing
	// declares this", never "no".
	Vision        bool        `json:"vision,omitempty"`
	Efforts       []string    `json:"efforts,omitempty"`
	Effort        string      `json:"effort,omitempty"`
	ContextWindow int         `json:"contextWindow,omitempty"`
	Price         *modelPrice `json:"price,omitempty"`
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
