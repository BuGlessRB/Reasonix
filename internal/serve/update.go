package serve

import "net/http"

// UpdateHost is the desktop application this kernel runs inside, as far as the
// hub is concerned. Nil leaves the update routes unregistered rather than
// answering for an application nobody here owns: a process that cannot bring
// one to a replaceable state and start its successor has no business declaring
// a replacement healthy either.
type UpdateHost interface {
	// AcknowledgeLaunchHealth retires the update this launch booted from. It
	// takes nothing: which transaction that is was read at startup, so the
	// caller says only that the launch it is in is working.
	AcknowledgeLaunchHealth() error
}

const codeUpdateRejected = "update.rejected"

func (h *Hub) registerUpdateRoutes(mux *http.ServeMux) {
	if h.opts.Update == nil {
		return
	}
	mux.HandleFunc("POST /update/health", h.acknowledgeLaunchHealth)
}

// The swap is performed by a process that cannot judge it. This is the other
// half: the application that booted from the replacement says it is working,
// and only then is the rollback material retired.
func (h *Hub) acknowledgeLaunchHealth(w http.ResponseWriter, r *http.Request) {
	if err := h.opts.Update.AcknowledgeLaunchHealth(); err != nil {
		refuse(w, http.StatusConflict, codeUpdateRejected, err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
