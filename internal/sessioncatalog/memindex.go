package sessioncatalog

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agent"
)

// MemIndex answers the catalog's session reads from memory instead of a
// database file. Scanning a directory's sidecars costs about the same as one
// reconcile pass and leaves nothing on disk to fall out of step with it, so a
// rebuild is the repair path for anything it could have missed.
type MemIndex struct {
	mu       sync.RWMutex
	byDir    map[string][]SessionRecord
	revision uint64
	now      func() time.Time
}

func NewMemIndex(now func() time.Time) *MemIndex {
	if now == nil {
		now = time.Now
	}
	return &MemIndex{byDir: map[string][]SessionRecord{}, now: now}
}

// ScanDirectory replaces one directory's records with what is on disk now.
func (m *MemIndex) ScanDirectory(target DirectoryTarget) error {
	target.Path = filepath.Clean(strings.TrimSpace(target.Path))
	if target.Path == "." || target.Path == "" {
		return nil
	}
	target.Scope, target.WorkspaceRoot = normalizeScope(target.Scope, target.WorkspaceRoot)
	ordered, err := agent.ListSessionOrder(target.Path)
	if err != nil {
		return err
	}
	records := make([]SessionRecord, 0, len(ordered))
	for _, info := range ordered {
		records = append(records, recordFromOrder(target, info))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byDir[target.Path] = records
	m.revision++
	return nil
}

// Revision changes whenever a scan lands, which is what invalidates a cursor.
func (m *MemIndex) Revision() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.revision
}

func (m *MemIndex) GetSession(path string) (SessionRecord, bool) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return SessionRecord{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, records := range m.byDir {
		for _, record := range records {
			if record.Path == path {
				return record, true
			}
		}
	}
	return SessionRecord{}, false
}

func (m *MemIndex) ListSessions(req SessionPageRequest) (SessionPage, error) {
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}
	if req.Limit > MaxLimit {
		req.Limit = MaxLimit
	}
	cursor, err := decodeSessionCursor(req.Cursor)
	if err != nil {
		return SessionPage{Items: []SessionRecord{}}, err
	}
	m.mu.RLock()
	revision := m.revision
	dirs := make([][]SessionRecord, 0, len(m.byDir))
	for _, records := range m.byDir {
		dirs = append(dirs, records)
	}
	m.mu.RUnlock()

	out := SessionPage{Items: []SessionRecord{}, Revision: revision}
	if cursor != nil && cursor.Revision != revision {
		out.StaleCursor = true
		return out, nil
	}
	match, err := sessionMatcher(req, m.now())
	if err != nil {
		return out, err
	}
	matched := make([]SessionRecord, 0, 64)
	for _, records := range dirs {
		for _, record := range records {
			if match(record) && afterCursor(record, cursor) {
				matched = append(matched, record)
			}
		}
	}
	slices.SortFunc(matched, func(a, b SessionRecord) int {
		if a.LastActivityAt != b.LastActivityAt {
			return int(b.LastActivityAt - a.LastActivityAt)
		}
		return strings.Compare(a.Path, b.Path)
	})
	if len(matched) > req.Limit {
		matched = matched[:req.Limit]
		last := matched[len(matched)-1]
		out.NextCursor = encodeSessionCursor(sessionPageCursor{
			Revision: revision, Activity: last.LastActivityAt, Path: last.Path,
		})
	}
	out.Items = matched
	return out, nil
}

// afterCursor is the keyset predicate the SQL page uses: strictly older, or the
// same instant further along in path order.
func afterCursor(record SessionRecord, cursor *sessionPageCursor) bool {
	if cursor == nil {
		return true
	}
	if record.LastActivityAt != cursor.Activity {
		return record.LastActivityAt < cursor.Activity
	}
	return record.Path > cursor.Path
}

// sessionMatcher mirrors the WHERE clause of the SQL page.
func sessionMatcher(req SessionPageRequest, now time.Time) (func(SessionRecord) bool, error) {
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	root := strings.TrimSpace(req.WorkspaceRoot)
	switch scope {
	case "", "all", "project", "global":
	default:
		return nil, fmt.Errorf("invalid session catalog scope %q", req.Scope)
	}
	directory := strings.TrimSpace(req.Directory)
	if directory != "" {
		directory = filepath.Clean(directory)
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	from, to := sessionTimeWindow(req.TimeFilter, now)
	return func(r SessionRecord) bool {
		switch {
		case r.MissingSince != 0 || r.Health == HealthMissing:
			return false
		// A conflict copy is a rescued file, not a conversation the user started.
		case r.Recovered:
			return false
		case scope == "project" && (r.Scope != "project" || r.WorkspaceRoot != root):
			return false
		case scope == "global" && r.Scope != "global":
			return false
		case directory != "" && r.Directory != directory:
			return false
		case from > 0 && r.LastActivityAt < from:
			return false
		case to > 0 && r.LastActivityAt >= to:
			return false
		}
		if query == "" {
			return true
		}
		for _, field := range []string{r.CustomTitle, r.Preview, r.TopicTitle, r.TopicID} {
			if strings.Contains(strings.ToLower(field), query) {
				return true
			}
		}
		return false
	}, nil
}

// sessionTimeWindow returns the half-open [from, to) the filter selects, with 0
// meaning unbounded on that side.
func sessionTimeWindow(filter string, now time.Time) (from, to int64) {
	value := strings.ToLower(strings.TrimSpace(filter))
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch value {
	case "", "all":
		return 0, 0
	case "today":
		return startToday.UnixMilli(), 0
	case "yesterday":
		return startToday.AddDate(0, 0, -1).UnixMilli(), startToday.UnixMilli()
	case "older":
		return 0, startToday.AddDate(0, 0, -1).UnixMilli()
	default:
		return timeFilterCutoff(value, now), 0
	}
}
