// Session switching over HTTP: /resume binds this runtime to another
// transcript. It lives apart from serve.go because the swap has three owners to
// keep in step — the controller's rotation gate, the session lease, and the
// broadcaster's ledger — and getting them out of order is how one conversation
// ends up written into another.
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
	"reasonix/internal/control"
	"reasonix/internal/store"
)

// resume loads a previous session from a JSONL file.
func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	dir := s.ctl().SessionDir()
	if dir == "" {
		http.Error(w, "sessions disabled", http.StatusBadRequest)
		return
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		http.Error(w, "invalid session dir", http.StatusBadRequest)
		return
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		http.Error(w, "invalid session dir", http.StatusBadRequest)
		return
	}
	absPath, err := filepath.Abs(strings.TrimSpace(body.Path))
	if err != nil || !store.IsSessionTranscriptName(filepath.Base(absPath)) {
		http.Error(w, "invalid session path", http.StatusBadRequest)
		return
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		http.Error(w, "invalid session path", http.StatusBadRequest)
		return
	}
	if realPath == realDir || !strings.HasPrefix(realPath, realDir+string(os.PathSeparator)) {
		http.Error(w, "path outside session dir", http.StatusForbidden)
		return
	}
	if agent.IsCleanupPending(realPath) {
		http.Error(w, "session is pending cleanup", http.StatusBadRequest)
		return
	}
	// Serialize with /new, /fork, and switchModel so the controller and lease
	// cannot land on different sessions. Validate first to avoid slow holders.
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	// Snapshot the current session before switching away — while this process
	// still holds its lease.
	if err := s.ctl().Snapshot(); err != nil {
		slog.Warn("serve: snapshot before resume", "err", err)
	}
	// Refuse to bind a session another runtime is writing (a desktop window,
	// another CLI); on success the lease now guards the resume target.
	if s.leases != nil {
		if err := s.leases.Rebind(realPath); err != nil {
			if errors.Is(err, agent.ErrSessionLeaseHeld) {
				http.Error(w, sessionInUseError(err), http.StatusConflict)
			} else {
				http.Error(w, "session lease: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	loaded, err := agent.LoadSession(realPath)
	if err != nil {
		// The lease already moved to the target; re-point it at the session the
		// controller still owns (best-effort).
		_ = s.rebindSessionLease(s.ctl().SessionPath())
		http.Error(w, "load session: "+err.Error(), http.StatusBadRequest)
		return
	}
	if hook := resumeBindHookForTest; hook != nil {
		hook()
	}
	if err := s.ctl().Resume(loaded, realPath); err != nil {
		// The turn in flight owns the session it is writing to. Binding another
		// one under it is how the running conversation's output lands in the
		// transcript the user just switched to.
		_ = s.rebindSessionLease(s.ctl().SessionPath())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if ctrl, ok := s.ctl().(*control.Controller); ok && s.leases != nil {
		if err := s.leases.BindControllerAuthority(ctrl); err != nil {
			http.Error(w, "session authority: unable to bind resumed session", http.StatusInternalServerError)
			return
		}
	}
	s.bc.ResetSession()
	w.WriteHeader(http.StatusNoContent)
}
