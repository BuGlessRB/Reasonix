package control

import (
	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// EnsureSessionPath pins a fresh auto-save file for this controller when none is
// set yet and a session dir is configured — the "fresh session" branch every
// surface runs right after building a controller. It is a no-op once a resume or
// continue has already pinned a path (SessionPath() != ""), so callers can run a
// conditional Resume and then invoke this unconditionally. Centralises the
// per-surface copies of this logic (the CLI chat/serve fresh branches and the
// bot's former ensureControllerSessionPath).
func (c *Controller) EnsureSessionPath() {
	if c.SessionPath() != "" || c.SessionDir() == "" {
		return
	}
	c.SetFreshSessionPath(agent.NewSessionPath(c.SessionDir(), c.Label()))
}

// Resume seeds the session from a loaded transcript and pins auto-save to it.
// Arriving from a different path rotates the private temporary generation; a
// same-path resume (rebuild migration) keeps it. It claims the rotation gate
// NewSession and Fork claim: a live turn owns the session it writes to, and
// Resume was the one swap that bound another one under it.
func (c *Controller) Resume(s *agent.Session, path string) error {
	if err := c.beginRotation(); err != nil {
		return err
	}
	defer c.endRotation()
	c.resume(s, path, true)
	return nil
}

// AdoptHistory makes a freshly built controller continue an existing
// conversation in path: it resumes the carried messages there when there are
// any, otherwise just points auto-save at path. An empty path with no messages
// is a no-op. This is the shared kernel of the model/effort switch across the
// CLI, the HTTP server, and ACP — each computes path its own way
// (ContinueSessionPath for the CLI/serve, the pinned transcript for ACP) and
// hands the carried history (Controller.History()) here. Keeping the
// Resume/SetSessionPath choice in one place avoids the orphaned-duplicate class
// of bug (#2807) recurring as each surface copied it.
func (c *Controller) AdoptHistory(msgs []provider.Message, path string) {
	if len(msgs) > 0 {
		if path != "" {
			if loaded, err := agent.LoadSession(path); err == nil && loaded != nil {
				if resumed, ok := loaded.CloneWithMessagesIfCompatible(msgs); ok {
					c.resume(resumed, path, false)
					return
				}
			}
		}
		c.resume(agent.NewSession("").CloneWithMessages(msgs), path, false)
	} else if path != "" {
		// Even an empty transcript can carry session-scoped sidecars such as a
		// running or blocked Goal. Resume a persisted empty session so controller
		// rebuilds preserve that state; fall back to a plain binding for a fresh
		// path that has not been saved yet.
		if loaded, err := agent.LoadSession(path); err == nil && loaded != nil {
			c.resume(loaded, path, false)
			return
		}
		c.SetSessionPath(path)
	}
}
