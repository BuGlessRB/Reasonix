package serve

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"reasonix/internal/control"
)

// Images ride into a turn as path references, exactly as they do from the CLI.
// A browser client cannot write that file itself, so this is the one step it
// needs the host for. The body is JSON rather than raw bytes because csrfGuard
// admits nothing else, and that guard is what keeps a cross-site form from
// posting here at all.
const maxAttachmentUpload = 10 << 20

func (s *Server) attachments(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, (maxAttachmentUpload/3)*4+1024)
	var body struct {
		Mime string `json:"mime"`
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
	saved, err := control.SaveImageBytesInRoot(root, body.Mime, raw)
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
	writeJSON(w, map[string]string{"path": rel, "ref": "@" + rel})
}
