// fail.go — how a refusal reaches a frontend: as a code, not as a sentence.
package serve

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Reason is a machine-readable refusal: the kernel decides what happened, a
// frontend decides how to say it. Message is English fallback for logs and for
// clients with no wording for this code — never the translation. Params carry
// the pieces a sentence needs, so grammar stays the frontend's problem.
type Reason struct {
	Code    string         `json:"code"`
	Message string         `json:"error"`
	Params  map[string]any `json:"params,omitempty"`
}

// refuse writes a Reason. Codes are dotted and stable: renaming one is a
// breaking change to every frontend, the same as renaming a route.
func refuse(w http.ResponseWriter, status int, code, message string, params map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Reason{Code: code, Message: message, Params: params})
}

// busy is the refusal a frontend has to phrase most carefully: nothing is
// wrong, the answer is "not while this is running". It is one helper because
// every caller of it means the same thing. Params carry what the reader needs
// in order to act — "close the pane first" is only useful alongside which one.
func busy(w http.ResponseWriter, code, message string, params map[string]any) {
	refuse(w, http.StatusConflict, code, message, params)
}

// coded carries a Reason out through call sites that only return `error`.
// "A turn is running" is known where the controller is, not at the mux, and
// this is what gets that decision to the boundary without the layers between
// learning about HTTP.
type coded struct {
	reason Reason
	status int
	err    error
}

func (c *coded) Error() string { return c.err.Error() }
func (c *coded) Unwrap() error { return c.err }

// refusal builds an error that writeErr will render as a Reason.
func refusal(status int, code string, err error, params map[string]any) error {
	return &coded{reason: Reason{Code: code, Message: err.Error(), Params: params}, status: status, err: err}
}

// busyErr is the "not while this is running" refusal in error form.
func busyErr(code, message string) error {
	return refusal(http.StatusConflict, code, errors.New(message), nil)
}

// writeErr turns an error into a response. One carrying a code reaches the
// frontend as one; anything else keeps the old shape, so adoption is
// incremental rather than a flag day.
func writeErr(w http.ResponseWriter, fallback int, err error) {
	var c *coded
	if errors.As(err, &c) {
		refuse(w, c.status, c.reason.Code, c.reason.Message, c.reason.Params)
		return
	}
	http.Error(w, err.Error(), fallback)
}
