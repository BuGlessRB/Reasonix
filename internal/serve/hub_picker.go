package serve

import (
	"encoding/json"
	"errors"
	"net/http"
)

var errFolderPickerUnsupported = errors.New("native folder picker is unavailable")

// Kept as a variable so the transport contract can be tested without opening
// a real operating-system dialog. The implementation is selected per OS.
var openLocalFolderPicker = pickLocalFolder

func (h *Hub) pickLocalFolderHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StartIn string `json:"startIn"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	path, err := openLocalFolderPicker(r.Context(), req.StartIn)
	if errors.Is(err, errFolderPickerUnsupported) {
		http.Error(w, err.Error(), http.StatusNotImplemented)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Path string `json:"path"`
	}{Path: path})
}
