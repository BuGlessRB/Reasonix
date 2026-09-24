package serve

import (
	"errors"
	"net/http"

	"reasonix/internal/contract/config"
	"reasonix/internal/platform/editor"
)

// AllowEditorOpen grants POST /workspace/editor. Off until a host asks for it:
// the editor launches on the machine running the kernel, so a server reached
// over the network would open a window nobody is sitting at. The desktop shell
// asks because its only client is its own window.
func (s *Server) AllowEditorOpen() { s.grants.editorOpen = true }

const (
	codeEditorMissing  = "editor.not_installed"
	codeEditorRefused  = "editor.launch_failed"
	codeEditorNoWindow = "editor.no_window"
)

// openInEditor opens this runtime's workspace, not a path the client names:
// the kernel already knows which directory it is driving, and taking one from
// the request would make this a way to open anything on the machine.
func (s *Server) openInEditor(w http.ResponseWriter, _ *http.Request) {
	if !s.grants.editorOpen {
		refuse(w, http.StatusForbidden, codeEditorNoWindow, "this kernel has no window to open an editor from", nil)
		return
	}
	root := s.ctl().WorkspaceRoot()
	spec, err := editor.Discover(config.LoadForEdit(config.UserConfigPath()).DesktopEditor())
	if err != nil {
		refuse(w, http.StatusNotFound, codeEditorMissing, err.Error(), nil)
		return
	}
	if err := editor.Open(spec, root); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, editor.ErrNoEditor) {
			status = http.StatusNotFound
		}
		refuse(w, status, codeEditorRefused, err.Error(), map[string]any{"editor": spec.Name})
		return
	}
	writeJSON(w, map[string]any{"editor": spec.Name, "root": root})
}
