package serve

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"reasonix/internal/memory"
)

// memoryEntry is one saved fact. Activation is the field that matters most to a
// reader and the one the type does not imply: pinned facts are in every prompt,
// relevant ones only surface when the turn looks related.
type memoryEntry struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Body        string `json:"body,omitempty"`
	Type        string `json:"type,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Activation  string `json:"activation"`
	Path        string `json:"path,omitempty"`
	Revision    int    `json:"revision,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Expired     bool   `json:"expired,omitempty"`
	// UsedLastTurn is why this endpoint is worth opening: it connects a stored
	// fact to the behaviour the user just saw.
	UsedLastTurn bool   `json:"usedLastTurn,omitempty"`
	Why          string `json:"why,omitempty"`
}

// memories lists saved facts with the last turn's recall folded in. Which facts
// exist is the less useful half; which ones were just acted on is the question a
// user actually has when the agent does something surprising.
func (s *Server) memories(w http.ResponseWriter, r *http.Request) {
	ctl := s.ctl()
	set := ctl.Memory()
	if set == nil {
		writeJSON(w, map[string]any{"memories": []memoryEntry{}, "recallQuery": ""})
		return
	}
	recall := ctl.LastMemoryRecall()
	used := make(map[string]string, len(recall.Hits))
	for _, hit := range recall.Hits {
		used[hit.Memory.Name] = strings.TrimSpace(hit.Reason)
	}
	raw := set.Store.ListAll()
	now := time.Now()
	out := make([]memoryEntry, 0, len(raw))
	for _, m := range raw {
		reason, hit := used[m.Name]
		out = append(out, memoryEntry{
			Name: m.Name, Title: m.Title, Description: m.Description, Body: m.Body,
			Type: string(m.Type), Scope: string(m.Scope),
			Activation: string(memory.ResolveActivation(m)),
			Path:       set.Store.Path(m.Name), Revision: m.Revision,
			CreatedAt: stamp(m.CreatedAt), UpdatedAt: stamp(m.UpdatedAt),
			Expired:      !m.ExpiresAt.IsZero() && m.ExpiresAt.Before(now),
			UsedLastTurn: hit, Why: reason,
		})
	}
	writeJSON(w, map[string]any{
		"memories":    out,
		"recallQuery": recall.Query,
		"indexPath":   set.Store.Path("MEMORY"),
	})
}

// forgetMemory archives one fact. Archiving rather than deleting is the store's
// own choice and the right one here: a fact removed by mistake is recoverable.
func (s *Server) forgetMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		missingField(w, "name")
		return
	}
	if err := s.ctl().ForgetMemory(strings.TrimSpace(body.Name)); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}
