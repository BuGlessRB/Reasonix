// boundary.go — the tool boundary as HTTP: the approval rules, and the jail an
// approved call runs inside.
package serve

import (
	"encoding/json"
	"net/http"

	"reasonix/internal/control"
)

func (s *Server) registerBoundaryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /permissions", s.permissions)
	mux.HandleFunc("POST /permissions", s.savePermissions)
	mux.HandleFunc("GET /sandbox", s.sandboxSettings)
	mux.HandleFunc("POST /sandbox", s.saveSandboxSettings)
	// The file every one of these is written to, for when it is the thing that
	// is wrong: each save above refuses with the same code, and this is where a
	// surface reads it before trying, and repairs it after.
	s.registerConfigFileRoutes(mux)
}

func (s *Server) permissions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().PermissionRules())
}

// savePermissions writes the deny/ask/allow lists and rebuilds. These decide
// what the agent may do to the files of the machine running the kernel, so they
// ride the same grant as the other host-owned settings: a networked client must
// not be able to widen its own boundary.
func (s *Server) savePermissions(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		refuse(w, http.StatusForbidden, "permissions.editing_disabled", "permission editing is not enabled on this server", nil)
		return
	}
	var body control.PermissionLists
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if err := s.ctl().SavePermissionRules(body); err != nil {
		saveFailed(w, http.StatusBadRequest, "permissions.rejected", err)
		return
	}
	if err := s.rebuildInPlace(r.Context()); err != nil {
		rebuildFailed(w, err)
		return
	}
	writeJSON(w, s.ctl().PermissionRules())
}

func (s *Server) sandboxSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().SandboxSettings())
}

func (s *Server) saveSandboxSettings(w http.ResponseWriter, r *http.Request) {
	if !s.grants.providerEdit {
		refuse(w, http.StatusForbidden, "sandbox.editing_disabled", "sandbox editing is not enabled on this server", nil)
		return
	}
	var body control.SandboxSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if err := s.ctl().SaveSandboxSettings(body); err != nil {
		saveFailed(w, http.StatusBadRequest, "sandbox.rejected", err)
		return
	}
	if err := s.rebuildInPlace(r.Context()); err != nil {
		rebuildFailed(w, err)
		return
	}
	writeJSON(w, s.ctl().SandboxSettings())
}
