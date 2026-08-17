package serve

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/store"
	"reasonix/internal/worktree"
)

// treeSessionsPerWorkspace bounds one workspace's branch of the tree. A long
// history is paged by opening the workspace, not by shipping all of it to a
// sidebar that shows a dozen rows.
const treeSessionsPerWorkspace = 50

// treeWorkspace is one folder in the sidebar: what it is called, whether a pane
// is driving it, and the conversations saved under it.
type treeWorkspace struct {
	Root string `json:"root"`
	Name string `json:"name"`
	// Isolated marks a delivery worktree. Its folder name is the project's, so
	// without this the sidebar shows two rows with one name.
	Isolated bool          `json:"isolated,omitempty"`
	Missing  bool          `json:"missing,omitempty"`
	Open     bool          `json:"open,omitempty"`
	Sessions []treeSession `json:"sessions"`
}

// treeSession is one conversation. RuntimeID is set when a pane already has it
// open, which is what lets the sidebar focus that pane instead of opening a
// second writer for the same transcript.
type treeSession struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	Turns     int    `json:"turns,omitempty"`
	RuntimeID string `json:"runtimeId,omitempty"`
}

func (h *Hub) registerTreeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /tree", h.tree)
	mux.HandleFunc("POST /tree/workspaces", h.addWorkspace)
	mux.HandleFunc("POST /tree/workspaces/remove", h.removeWorkspace)
	mux.HandleFunc("POST /tree/sessions/remove", h.removeSession)
	mux.HandleFunc("POST /tree/sessions/rename", h.renameSession)
}

// tree answers the whole sidebar in one request: every remembered workspace
// with the sessions saved under it. Titles come from each project's cache and
// are never generated here — the pane that opens a session does that.
func (h *Hub) tree(w http.ResponseWriter, _ *http.Request) {
	open := h.openSessions()
	roots := h.roots()
	out := make([]treeWorkspace, 0, len(roots))
	for _, root := range roots {
		// Open means a pane is driving this folder — true from the moment one is
		// opened, before its first turn has written a transcript to list.
		node := treeWorkspace{
			Root: root, Name: filepath.Base(root), Open: h.rootRuntime(root) != nil,
			Isolated: worktree.IsManagedPath(root, config.DeliveryWorktreeDir()),
			Sessions: []treeSession{},
		}
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			node.Missing = true
		}
		node.Sessions = append(node.Sessions, h.workspaceSessions(root, open)...)
		out = append(out, node)
	}
	writeJSON(w, out)
}

// workspaceSessions lists one folder's conversations, hiding what the pickers
// hide: subagent traces and redundant recovery copies. A copy that a pane has
// open stays, because something is driving it.
func (h *Hub) workspaceSessions(root string, open map[string]string) []treeSession {
	dir := SessionDirFor(root)
	listed, err := agent.ListSessions(dir)
	if err != nil {
		return nil
	}
	titles := h.titleCacheFor(dir)
	out := make([]treeSession, 0, len(listed))
	for _, si := range listed {
		if len(out) >= treeSessionsPerWorkspace {
			break
		}
		base := filepath.Base(si.Path)
		if store.IsSubagentTranscriptName(base) {
			continue
		}
		runtimeID := open[agent.CanonicalSessionPath(si.Path)]
		if runtimeID == "" && hiddenRecoveryCopy(si, "") {
			continue
		}
		name := strings.TrimSuffix(base, ".jsonl")
		// A name the user typed outranks the generated one; without this a
		// rename would be written to the sidecar and never show up.
		title := strings.TrimSpace(si.CustomTitle)
		if title == "" {
			title, _ = titles.get(name+".jsonl", titleSource(si.Preview), agent.SessionContentModTime(si.Path).UnixNano())
		}
		if title == "" {
			title = previewTitle(si.Preview)
		}
		out = append(out, treeSession{
			Path: si.Path, Name: name, Title: title, Turns: si.Turns, RuntimeID: runtimeID,
		})
	}
	return out
}

// addWorkspace remembers a folder without opening it. Adding and opening are
// separate acts: the sidebar lists what you work on, panes are what you drive.
func (h *Hub) addWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	dir, err := resolveWorkspaceDir(strings.TrimSpace(body.Path))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rememberWorkspace(dir)
	writeJSON(w, treeWorkspace{Root: dir, Name: filepath.Base(dir), Sessions: []treeSession{}})
}

// removeWorkspace drops a folder from the sidebar. Nothing on disk is touched —
// the sessions stay where they are and the folder can be added back.
func (h *Hub) removeWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	dir := strings.TrimSpace(body.Path)
	if dir == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	if rt := h.rootRuntime(dir); rt != nil {
		busy(w, "workspace.has_open_panes", "close this workspace's panes first")
		return
	}
	forgetWorkspace(dir)
	w.WriteHeader(http.StatusNoContent)
}

// removeSession deletes a conversation the sidebar lists. A session a pane is
// driving is refused rather than pulled out from under it: the pane owns the
// teardown of its own jobs, so closing it first is what makes this safe.
func (h *Hub) removeSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	path, err := filepath.Abs(strings.TrimSpace(body.Path))
	if err != nil || !store.IsSessionTranscriptName(filepath.Base(path)) {
		http.Error(w, "invalid session path", http.StatusBadRequest)
		return
	}
	if h.openSessions()[agent.CanonicalSessionPath(path)] != "" {
		busy(w, "session.has_open_pane", "close this session's pane first")
		return
	}
	dir := filepath.Dir(path)
	if !h.ownsSessionDir(dir) {
		refuse(w, http.StatusForbidden, "session.outside_workspace", "path outside a known workspace", nil)
		return
	}
	// A pane's current path is narrower than "anyone writing this file": a
	// recovery branch or a mid-rotation session is held without being one.
	// Taking the guard beats probing it, which leaves a window for a writer.
	guard, err := agent.TryAcquireSessionRemovalGuard(path)
	if err != nil {
		var held *agent.SessionLeaseError
		if errors.As(err, &held) {
			busy(w, "session.in_use", "this conversation is still being written to")
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := removeSessionFiles(dir, path); err != nil {
		guard.Release()
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := guard.RemoveSidecarsAndRelease(); err != nil {
		// The conversation is already gone; a surviving lock file is stale
		// bookkeeping, not a failed delete.
		slog.Warn("serve: session removed, lock files survived", "path", path, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// renameSession sets the name a session shows under. Unlike removal this is
// safe on an open one — the title lives in the sidecar, not the transcript — so
// a tab can be renamed without closing the pane behind it.
func (h *Hub) renameSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	path, err := filepath.Abs(strings.TrimSpace(body.Path))
	if err != nil || !store.IsSessionTranscriptName(filepath.Base(path)) {
		http.Error(w, "invalid session path", http.StatusBadRequest)
		return
	}
	if !h.ownsSessionDir(filepath.Dir(path)) {
		refuse(w, http.StatusForbidden, "session.outside_workspace", "path outside a known workspace", nil)
		return
	}
	if err := agent.RenameSession(path, body.Title); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownsSessionDir reports whether dir is the session directory of a workspace
// this hub lists, which is the boundary a delete may not reach past.
func (h *Hub) ownsSessionDir(dir string) bool {
	for _, root := range h.roots() {
		if known, err := filepath.Abs(SessionDirFor(root)); err == nil && known == dir {
			return true
		}
	}
	return false
}

// roots returns the sidebar's folders: everything remembered, plus whatever a
// pane is driving, so an open workspace can never be missing from the tree.
func (h *Hub) roots() []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		out = append(out, dir)
	}
	for _, rt := range h.Runtimes() {
		add(rt.Server.Controller().WorkspaceRoot())
	}
	for _, dir := range Workspaces() {
		add(dir)
	}
	return out
}

// openSessions maps canonical session paths to the runtime driving them.
func (h *Hub) openSessions() map[string]string {
	out := map[string]string{}
	for _, rt := range h.Runtimes() {
		if path := agent.CanonicalSessionPath(rt.Server.Controller().SessionPath()); path != "" {
			out[path] = rt.ID
		}
	}
	return out
}

func (h *Hub) rootRuntime(root string) *Runtime {
	for _, rt := range h.Runtimes() {
		if rt.Server.Controller().WorkspaceRoot() == root {
			return rt
		}
	}
	return nil
}

// titleCacheFor keeps one reader per project directory. Titles are file-name
// keyed and unique only within a project, so the caches must not be shared.
func (h *Hub) titleCacheFor(dir string) *titleCache {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.titles == nil {
		h.titles = map[string]*titleCache{}
	}
	if c := h.titles[dir]; c != nil {
		return c
	}
	c := newTitleCache(dir)
	h.titles[dir] = c
	return c
}

// workspaceRootForSession recovers which folder a transcript belongs to from
// its own sidecar, for an open request that names a session but no root.
func workspaceRootForSession(path string) string {
	meta, ok, err := agent.LoadBranchMeta(strings.TrimSpace(path))
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(meta.WorkspaceRoot)
}
