// refuse_common.go — the refusals many endpoints share, so one kind of failure
// is one code rather than one per handler.
package serve

import (
	"encoding/json"
	"net/http"
)

// The recurring kinds. A frontend that has wording for these can say most
// failures without a code per endpoint, which is what 172 hand-written
// http.Error calls cost: the same "bad body" phrased twelve ways, none of them
// translatable.
const (
	codeBadBody        = "request.bad_body"
	codeMissingField   = "request.missing_field"
	codeNotFound       = "request.not_found"
	codeUnknownProject = "project.unknown"
	codeSessionInUse   = "busy.session_in_use"
)

// badBody refuses a request whose body did not parse. The parse error itself is
// not shown: it names Go types and offsets, which tells a reader nothing and a
// translator less.
func badBody(w http.ResponseWriter) {
	refuse(w, http.StatusBadRequest, codeBadBody, "the request body could not be read", nil)
}

// missingField refuses a request that left out something required. The field
// travels as a param so the sentence around it stays the frontend's.
func missingField(w http.ResponseWriter, field string) {
	refuse(w, http.StatusBadRequest, codeMissingField, "missing "+field, map[string]any{"field": field})
}

// notFound refuses a name the runtime does not know: what kind of thing was
// asked for, and which one.
func notFound(w http.ResponseWriter, kind, name string) {
	refuse(w, http.StatusNotFound, codeNotFound, "no "+kind+" named "+name,
		map[string]any{"kind": kind, "name": name})
}

// unknownProject refuses a root this runtime has not opened. It is not a 404:
// the project may well exist on disk, and asking about one that is not open
// here is a request that cannot be served, not a thing that is missing.
func unknownProject(w http.ResponseWriter, root string) {
	refuse(w, http.StatusBadRequest, codeUnknownProject, "project is not open here",
		map[string]any{"root": root})
}

// sessionInUse is the refusal a reader has to be able to act on: the session is
// held somewhere else, and the detail says where. It is a conflict, not a
// failure — nothing is broken and nothing needs reporting.
func sessionInUse(w http.ResponseWriter, err error) {
	busy(w, codeSessionInUse, sessionInUseError(err), map[string]any{"detail": sessionInUseError(err)})
}

// decodeBody reads a JSON body under a size cap and refuses as one kind when it
// does not parse. Unknown fields are rejected: a client sending a field this
// build does not have is not saying what it thinks it is saying.
func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		badBody(w)
		return false
	}
	return true
}
