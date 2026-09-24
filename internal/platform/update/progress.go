package update

// The steps an install passes through beyond the download's own. Authorizing is
// the Linux package prompt; relaunching is the last thing a caller sees, because
// what follows it is this process ending.
const (
	PhaseIdle        = "idle"
	PhaseAuthorizing = "authorizing"
	PhaseRelaunching = "relaunching"
	PhaseFailed      = "error"
)

// Progress is what a version panel renders while an install is in flight. It
// is a projection with no id, no replay and no lifecycle: a missed frame is
// restored by the next read, and what an install ends with -- this process
// exiting -- is not a frame anyone receives. An event would owe delivery across
// the restart it is itself causing.
type Progress struct {
	Version  string `json:"version"`
	Phase    string `json:"phase"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Err      string `json:"err,omitempty"`
}

// Running reports whether an install is under way, which is the one thing a
// caller decides on rather than renders: a second install started over the
// first would have two writers for one set of files.
func (p Progress) Running() bool {
	switch p.Phase {
	case "", PhaseIdle, PhaseCached, PhaseFailed:
		return false
	}
	return true
}
