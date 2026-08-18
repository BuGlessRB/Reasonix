// session.go — which session a window opens on.
package main

import (
	"log/slog"

	"reasonix/internal/control"
	"reasonix/internal/serve"
)

// adoptWindowPane opens the window on the session it inherited, or on a new one
// when that session is still being written — by another window, or by one of our
// own processes taking its time to exit. A window inherits its session rather
// than naming one, so a held session is not an error to report but a session to
// stop wanting: writing into it anyway is what forks the transcript.
func adoptWindowPane(hub *serve.Hub, srv *serve.Server, bc *serve.Broadcaster, ctrl *control.Controller) error {
	_, err := hub.Adopt(srv, bc)
	if err == nil {
		return nil
	}
	slog.Warn("studio: the last session is still being written; opening a new one", "err", err)
	if err := ctrl.NewSession(); err != nil {
		return err
	}
	_, err = hub.Adopt(srv, bc)
	return err
}
