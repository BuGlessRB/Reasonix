package serve

import (
	"encoding/json"
	"fmt"
	"net/http"

	"reasonix/internal/control"
)

// maxDroppedPaths bounds one drop. The body limit alone would not: each path
// costs a symlink resolution and a stat, and a folder emptied onto the window
// is a plausible accident.
const maxDroppedPaths = 64

// droppedRef answers one path with the token to put in the line, or with why
// that path has none. Per-path rather than per-request: dropping six files of
// which one has since been moved should attach the five.
type droppedRef struct {
	Ref  string `json:"ref,omitempty"`
	Path string `json:"path,omitempty"`
	// Image says a text-only model will not see this one, which is the whole
	// reason the composer says so before the turn is sent.
	Image bool   `json:"image,omitempty"`
	Error string `json:"error,omitempty"`
}

// drop names what a dropped path is called inside a turn. The host has to say:
// whether a path is inside the workspace compares two spellings of a location,
// and the reference minted for anything outside must survive an @ grammar the
// page cannot see. A browser tab never reaches here — it holds bytes and no
// path, which is what POST /attachments is for.
func (s *Server) drop(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "read drop: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Paths) > maxDroppedPaths {
		http.Error(w, fmt.Sprintf("%d paths dropped at once; %d is the most one drop carries", len(body.Paths), maxDroppedPaths), http.StatusBadRequest)
		return
	}
	ctl := s.ctl()
	out := make([]droppedRef, 0, len(body.Paths))
	for _, path := range body.Paths {
		token, display, err := ctl.DroppedRef(path)
		if err != nil {
			out = append(out, droppedRef{Error: err.Error()})
			continue
		}
		out = append(out, droppedRef{Ref: "@" + token, Path: display, Image: control.RefIsImage(display)})
	}
	writeJSON(w, out)
}
