package serve

import (
	"encoding/json"
	"net/http"

	"reasonix/internal/checkpoint"
	"reasonix/internal/control"
)

// A rewind is two calls when the plan needs consent: POST /rewind refuses a
// partial-coverage plan outright, so prepare reports the gaps and commit applies
// the plan the user saw. The CLI picker has always worked this way.

// rewindScope maps the wire's scope name. Anything unrecognised means both,
// matching what the single-shot endpoint has always done.
func rewindScope(name string) control.RewindScope {
	switch name {
	case "code":
		return control.RewindCode
	case "conversation":
		return control.RewindConversation
	}
	return control.RewindBoth
}

func (s *Server) rewindPrepare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Turn  int    `json:"turn"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Turn < 0 {
		missingField(w, "turn")
		return
	}
	plan, err := s.ctl().PrepareRewind(body.Turn, rewindScope(body.Scope))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, struct {
		checkpoint.RewindPlan
		RequiresConfirmation bool `json:"requiresConfirmation"`
	}{RewindPlan: plan, RequiresConfirmation: control.RewindPlanRequiresConfirmation(plan)})
}

func (s *Server) rewindCommit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlanID string `json:"planId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlanID == "" {
		http.Error(w, "missing planId", http.StatusBadRequest)
		return
	}
	result, err := s.ctl().CommitRewind(body.PlanID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.ctl().Snapshot(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// rewindUndo reverses a committed rewind. The commit result carries the
// transaction id and whether this is available, so a frontend can offer it while
// the id is still in hand.
func (s *Server) rewindUndo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransactionID string `json:"transactionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TransactionID == "" {
		http.Error(w, "missing transactionId", http.StatusBadRequest)
		return
	}
	result, err := s.ctl().UndoRewind(body.TransactionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.ctl().Snapshot(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
