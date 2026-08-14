package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reasonix/internal/provider"
	"reasonix/internal/store"
)

// reconcileSessionDir runs the directory-wide passes: sidecar reconciliation,
// then reclamation of transcripts that hold no user message. Reclamation goes
// through the host's cleanup path, so a host that trashes deletions trashes
// these too rather than unlinking them.
func reconcileSessionDir(dir string, cleanup func(CleanupPendingInfo) error) error {
	errs := []error{ReconcileSessionSidecars(dir)}
	if cleanup == nil {
		return errors.Join(errs...)
	}
	empty, err := ReclaimableEmptySessions(dir, time.Now(), EmptySessionGracePeriod)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, path := range empty {
		info := CleanupPendingInfo{SessionPath: path, Meta: CleanupPendingMeta{Operation: "delete"}}
		if err := cleanup(info); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

// EmptySessionGracePeriod is how long a transcript holding no user message must
// sit idle before it may be reclaimed. It covers the gap between creating a
// session file and the first turn reaching it; a live one holds the write lease.
const EmptySessionGracePeriod = 5 * time.Minute

// ReclaimableEmptySessions returns transcripts in dir that hold no user message
// at all and have been idle for grace: v1 rotated the live log to
// <name>__archive_<ts> on /new, and aborted forks land the same way. Returns
// candidates only; disposal (trash, delete) is caller policy.
func ReclaimableEmptySessions(dir string, now time.Time, grace time.Duration) ([]string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !store.IsSessionTranscriptName(name) || store.IsSubagentTranscriptName(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if IsCleanupPending(path) || SessionLeaseHeld(path) {
			continue
		}
		mod := SessionContentModTime(path)
		if mod.IsZero() || now.Sub(mod) < grace {
			continue
		}
		if sessionMetaCountsTurns(path) || !sessionHoldsNoUserMessage(path) {
			continue
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

// sessionMetaCountsTurns reports a listing sidecar that claims user turns. It
// may only spare a candidate, never authorize one: the count is derived and can
// lag the transcript it describes.
func sessionMetaCountsTurns(path string) bool {
	meta, ok, err := LoadBranchMeta(path)
	if err != nil {
		return true
	}
	return ok && meta.Turns > 0
}

// sessionHoldsNoUserMessage reports that the transcript replays whole and holds
// no user-role message. An unreadable or damaged one answers false — what could
// not be read may be the user's.
func sessionHoldsNoUserMessage(path string) bool {
	msgs, _, damaged, err := loadSessionMessages(path)
	if err != nil || damaged {
		return false
	}
	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			return false
		}
	}
	return true
}
