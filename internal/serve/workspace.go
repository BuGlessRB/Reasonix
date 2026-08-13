package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/fileutil"
	"reasonix/internal/worktree"
)

const workspaceRecentMax = 12

// AllowWorkspaceSwitch grants POST /workspace. It is off until a host asks for
// it, and no config file can turn it on: a server reachable over the network
// would otherwise let any client repoint the agent at any directory it can
// read. The desktop shell asks because its only client is its own window.
func (s *Server) AllowWorkspaceSwitch() { s.allowWorkspaceSwitch = true }

// workspacesPath is where this frontend remembers the folders it has driven.
// Deliberately not the desktop app's list: that file doubles as its startup
// chdir pointer, and switching a project here must not move another app.
func workspacesPath() string {
	dir := config.MemoryUserDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "serve-workspaces.json")
}

// Workspaces is the most-recent-first list of folders this frontend has opened.
// Exported for the shell, which reopens the head of the list at launch.
func Workspaces() []string {
	p := workspacesPath()
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var paths []string
	if json.Unmarshal(data, &paths) != nil {
		return nil
	}
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func rememberWorkspace(dir string) {
	p := workspacesPath()
	if p == "" || dir == "" {
		return
	}
	paths := []string{dir}
	for _, path := range Workspaces() {
		if len(paths) >= workspaceRecentMax {
			break
		}
		if path != dir {
			paths = append(paths, path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return
	}
	if err := fileutil.AtomicWriteFile(p, data, 0o644); err != nil {
		slog.Warn("serve: remember workspace", "err", err)
	}
}

// SessionDirFor is where root's transcripts live. Exported because the shell
// has to build its first controller with the same answer a later switch uses,
// or the session rail would list one project's history while driving another.
func SessionDirFor(root string) string {
	if dir := config.ProjectSessionDir(root); dir != "" {
		return dir
	}
	return config.SessionDir()
}

func (s *Server) workspaces(w http.ResponseWriter, r *http.Request) {
	type workspaceView struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	current := s.ctl().WorkspaceRoot()
	recents := []workspaceView{}
	for _, p := range Workspaces() {
		if p == current {
			continue
		}
		recents = append(recents, workspaceView{Path: p, Name: filepath.Base(p)})
	}
	writeJSONCached(w, r, struct {
		Current  string          `json:"current"`
		Switch   bool            `json:"canSwitch"`
		Isolate  bool            `json:"canIsolate"`
		Recents  []workspaceView `json:"recents"`
		Isolated bool            `json:"isolated,omitempty"`
	}{
		Current:  current,
		Switch:   s.allowWorkspaceSwitch,
		Isolate:  s.allowWorkspaceSwitch && worktree.Inspect(r.Context(), current).Available,
		Recents:  recents,
		Isolated: worktree.IsManagedPath(current, config.DeliveryWorktreeDir()),
	})
}

// workspace repoints the whole runtime at another folder. The conversation is
// deliberately not carried: the transcript, the memory index, and the skill set
// all belong to the project that produced them.
func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	if !s.allowWorkspaceSwitch {
		http.Error(w, "this server may not change its workspace", http.StatusForbidden)
		return
	}
	var body struct {
		Path    string `json:"path"`
		Isolate bool   `json:"isolate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	s.bindMu.Lock()
	defer s.bindMu.Unlock()

	dir := strings.TrimSpace(body.Path)
	if body.Isolate {
		res, err := worktree.Create(r.Context(), s.ctl().WorkspaceRoot(), config.DeliveryWorktreeDir())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		dir = res.WorkspaceRoot
	}
	dir, err := resolveWorkspaceDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.switchWorkspaceLocked(r.Context(), dir); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, struct {
		WorkspaceRoot string `json:"workspaceRoot"`
	}{dir})
}

func resolveWorkspaceDir(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("missing path")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}

// switchWorkspaceLocked builds a replacement runtime rooted at dir and publishes
// it. bindMu must be held. The outgoing controller keeps serving until the
// replacement is fully built, so a failure leaves the session untouched.
func (s *Server) switchWorkspaceLocked(ctx context.Context, dir string) error {
	cur := s.ctl()
	if cur.WorkspaceRoot() == dir {
		return nil
	}
	if controllerHasActiveRuntimeWork(cur) {
		return fmt.Errorf("cannot change the workspace while active work or background jobs are running")
	}
	// The outgoing conversation stays in its own project's session dir; persist
	// it before letting go, because nothing carries it forward.
	if err := cur.Snapshot(); err != nil {
		slog.Warn("serve: snapshot before workspace switch", "err", err)
	}

	newCtrl, err := s.buildForWorkspace(ctx, dir, currentModelRef(cur))
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	newCtrl.EnableInteractiveApproval()
	newCtrl.SetOnSessionRecovered(sessionLeaseRecoveryHandler(s.leases))

	s.mu.Lock()
	if s.ctrl != cur {
		s.mu.Unlock()
		newCtrl.Close()
		return fmt.Errorf("session changed during the workspace switch")
	}
	s.ctrl = newCtrl
	s.mu.Unlock()

	// Session file names are timestamps, unique per project but not across
	// them, so the title cache has to move with the directory.
	s.titles.setDir(newCtrl.SessionDir())
	// No session file exists yet in the new project; releasing is what an empty
	// path means to the keeper.
	if err := s.rebindSessionLeaseFor(newCtrl.SessionPath(), newCtrl); err != nil {
		slog.Warn("serve: rebind session lease after workspace switch", "err", err)
	}
	s.bc.ResetSession()
	rememberWorkspace(dir)

	cur.Close()
	return nil
}

func (s *Server) buildForWorkspace(ctx context.Context, dir, ref string) (*control.Controller, error) {
	if s.buildWorkspaceController != nil {
		return s.buildWorkspaceController(ctx, dir, ref)
	}
	return boot.Build(ctx, boot.Options{
		Model:         ref,
		WorkspaceRoot: dir,
		SessionDir:    SessionDirFor(dir),
		Sink:          s.bc,
		Stderr:        os.Stderr,
		StatsSource:   "serve",
	})
}
