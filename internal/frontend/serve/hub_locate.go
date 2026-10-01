package serve

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	codeWorkspaceUnknown = "workspace.not_listed"
	codeWorkspaceMissing = "workspace.folder_missing"
)

// locateWorkspace answers where a project in the sidebar sits on this machine,
// for the shell to show in the file manager. It names only folders the window
// already lists, so the page cannot use it to ask about an arbitrary path.
func (h *Hub) locateWorkspace(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	if root == "" {
		missingField(w, "root")
		return
	}
	if !h.listsWorkspace(root) {
		refuse(w, http.StatusNotFound, codeWorkspaceUnknown, "that folder is not a project in this window", nil)
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		refuse(w, http.StatusNotFound, codeWorkspaceMissing, "the project folder is not on disk", nil)
		return
	}
	writeJSON(w, workspaceLocation{Path: abs, Dir: true})
}

func (h *Hub) listsWorkspace(root string) bool {
	if slices.Contains(LaunchWorkspaces(), root) {
		return true
	}
	return len(h.panesWhere(func(rt *Runtime) bool {
		return rt.Local() && rt.Server.Controller().WorkspaceRoot() == root
	})) > 0
}
