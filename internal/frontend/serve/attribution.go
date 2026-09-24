package serve

import (
	"net/http"

	"reasonix/internal/session/control"
)

// submitAs runs a submitted line as the one who sent it: a paired device's
// line lands as that device's, and the window's as its own.
func submitAs(ctrl control.SessionAPI, r *http.Request, input, format string) {
	if via := viaOf(r); via != nil {
		ctrl.SubmitHTTPFrom(input, format, via)
		return
	}
	ctrl.SubmitHTTPFormat(input, format)
}

// approveAs answers a prompt as the one who answered it. The device changes
// who the receipt names, never what the answer allows.
func approveAs(ctrl control.SessionAPI, r *http.Request, id string, allow, session, persist bool) {
	if via := viaOf(r); via != nil {
		ctrl.ApproveFrom(id, allow, session, persist, via)
		return
	}
	ctrl.Approve(id, allow, session, persist)
}
