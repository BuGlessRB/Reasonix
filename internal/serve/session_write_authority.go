package serve

import (
	"log/slog"
	"path/filepath"
	"strings"

	"reasonix/internal/control"
)

// SetSessionLeases hands the server the session-lease keeper that guards its
// active session file. Call it before serving; a nil keeper leaves gating off.
func (s *Server) SetSessionLeases(k *control.SessionLeaseKeeper) error {
	s.leases = k
	if ctrl, ok := s.ctl().(*control.Controller); ok {
		ctrl.SetOnSessionRecovered(sessionLeaseRecoveryHandler(k))
		ctrl.SetOnSessionPathChanged(s.followSessionLease)
		if k != nil {
			return k.BindControllerAuthority(ctrl)
		}
	}
	return nil
}

// followSessionLease takes the lease on a path the controller just moved to —
// a no-op where an endpoint already rebound (/resume, /new, /fork). What it
// covers is the path no endpoint minted: the one a controller creates for itself
// on the first turn, which used to stay unowned for the window's life. A refusal
// stands, and a save needing authority is then refused rather than forked.
func (s *Server) followSessionLease(path string) {
	if s == nil || s.leases == nil || strings.TrimSpace(path) == "" {
		return
	}
	if err := s.rebindSessionLease(path); err != nil {
		slog.Warn("serve: session is written by someone else; this pane holds no authority for it",
			"session", filepath.Base(path), "err", err)
	}
}

func sessionLeaseRecoveryHandler(k *control.SessionLeaseKeeper) func(control.SessionRecoveryInfo) error {
	if k == nil {
		return nil
	}
	return k.HandleSessionRecovered
}

// rebindSessionLease moves the server's session lease to path and rebinds the
// write authority generation. A nil keeper gates nothing (tests, embedded use).
func (s *Server) rebindSessionLease(path string) error {
	ctrl, _ := s.ctl().(*control.Controller)
	return s.rebindSessionLeaseFor(path, ctrl)
}

func (s *Server) rebindSessionLeaseFor(path string, ctrl *control.Controller) error {
	if s.leases == nil {
		return nil
	}
	if err := s.leases.Rebind(path); err != nil {
		return err
	}
	if ctrl != nil {
		return s.leases.BindControllerAuthority(ctrl)
	}
	return nil
}
