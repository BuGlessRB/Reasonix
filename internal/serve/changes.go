package serve

import (
	"encoding/json"
	"net/http"

	"reasonix/internal/gitstatus"
)

// changes reports what the working tree currently differs by. A frontend that
// derives its pending-change list from tool events instead keeps showing a file
// the agent created and a shell command then deleted, because no event says so.
// Repo is false for a workspace that is not version-controlled, which is a
// fallback signal and not a failure.
func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	list, ok, err := gitstatus.Status(r.Context(), s.ctl().WorkspaceRoot())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if list == nil {
		list = []gitstatus.Change{}
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Repo    bool               `json:"repo"`
		Changes []gitstatus.Change `json:"changes"`
	}{Repo: ok, Changes: list})
}
