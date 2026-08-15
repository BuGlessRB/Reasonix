// themes.go — installed theme packs and which one is active.
package serve

import (
	"encoding/json"
	"net/http"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/theme"
)

func (s *Server) registerThemeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /themes", s.themes)
	mux.HandleFunc("POST /themes", s.activateTheme)
}

type themeView struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Author      string                       `json:"author,omitempty"`
	Description string                       `json:"description,omitempty"`
	Active      bool                         `json:"active,omitempty"`
	Tokens      map[string]map[string]string `json:"tokens"`
}

// The list carries every pack's full token set, not just the active one's. A
// picker that has to fetch a pack before it can preview it cannot preview on
// hover, and the whole payload is a few hundred colours.
func (s *Server) themes(w http.ResponseWriter, r *http.Request) {
	active := ""
	if cfg, err := config.Load(); err == nil {
		active = strings.TrimSpace(cfg.Desktop.ThemePack)
	}
	packs := theme.List()
	out := make([]themeView, 0, len(packs))
	for _, p := range packs {
		out = append(out, themeView{
			ID: p.ID, Name: p.Name, Author: p.Author, Description: p.Description,
			Active: p.ID == active, Tokens: p.Tokens,
		})
	}
	writeJSONCached(w, r, out)
}

// An empty id is the default appearance, which is a real choice rather than a
// missing one: it is how the user turns a pack off.
func (s *Server) activateTheme(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ID)
	if id != "" {
		if _, err := theme.Load(id); err != nil {
			writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
	}
	// LoadForEdit keeps the user's file as they wrote it: only this field is
	// rewritten, so activating a theme never reformats the rest of the config.
	edit := config.LoadForEdit(config.UserConfigPath())
	edit.Desktop.ThemePack = id
	if err := edit.SaveTo(config.UserConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
