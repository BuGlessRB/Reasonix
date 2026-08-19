package serve

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"reasonix/internal/control"
)

// Bytes ride into a turn as path references, exactly as the CLI's do. This is
// the door for a client that has bytes and no path: a browser tab, and the
// clipboard everywhere. A window that knows the path uses POST /drop and copies
// nothing. JSON rather than raw bytes because csrfGuard admits nothing else.
// control enforces the real per-kind limit once these are decoded.
const maxAttachmentUpload = 25 << 20

func (s *Server) attachments(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, (maxAttachmentUpload/3)*4+1024)
	var body struct {
		Mime string `json:"mime"`
		// Name only supplies the extension the bytes are stored under; the
		// stored name is generated either way.
		Name string `json:"name"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "read attachment: "+err.Error(), http.StatusBadRequest)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		http.Error(w, "attachment data must be base64", http.StatusBadRequest)
		return
	}
	root := s.ctl().WorkspaceRoot()
	// The declared type picks the door, not the name: bytes claiming to be a
	// picture must prove it, because a turn referencing them asks a model to
	// look at them. Everything else is stored as the file it says it is.
	save := func() (string, error) { return control.SaveAttachmentBytesInRoot(root, body.Name, raw) }
	if body.Name == "" || strings.HasPrefix(strings.ToLower(body.Mime), "image/") {
		save = func() (string, error) { return control.SaveImageBytesInRoot(root, body.Mime, raw) }
	}
	saved, err := save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The turn parser resolves "@<path>"; hand back the exact token so the
	// client never reconstructs the reference syntax itself.
	rel := saved
	if r, relErr := filepath.Rel(root, saved); relErr == nil && !strings.HasPrefix(r, "..") {
		rel = filepath.ToSlash(r)
	}
	writeJSON(w, map[string]any{"path": rel, "ref": "@" + rel, "image": control.RefIsImage(rel)})
}
