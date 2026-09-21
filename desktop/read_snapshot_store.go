package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	readSnapshotMemory = 32 << 20
	readSnapshotDisk   = 256 << 20
	readSnapshotMax    = 64 << 20
	readSnapshotIdle   = 10 * time.Minute
	readSnapshotLife   = 30 * time.Minute
)

// A cursor owns a frozen result, never a position in a live ordering. The store
// is process-local and disposable; it does not change any durable user format.
type readSnapshotStore struct {
	mu           sync.Mutex
	buildWG      sync.WaitGroup
	entries      map[string]*readSnapshot
	building     map[string]*readSnapshotBuild
	gate         chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	closed       bool
	memory, disk int64
}

type readSnapshotBuild struct {
	done     chan struct{}
	snapshot *readSnapshot
	err      error
}

type readSnapshot struct {
	data               *readSnapshot
	leases             int
	removed            bool
	mu                 sync.Mutex
	id, binding        string
	created, used      time.Time
	rows               [][]byte
	db                 *sql.DB
	dir                string
	count              int
	size, memory, disk int64
	overhead           int64
	metadata           json.RawMessage
	validate           func() error
	released           bool
}

type readSnapshotCursor struct {
	Version int    `json:"v"`
	ID      string `json:"id"`
	Binding string `json:"q"`
	Offset  int    `json:"n"`
}

type ReadError struct {
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type ReadSnapshotDiagnostics struct {
	Handles           int   `json:"handles"`
	ActiveBuilders    int   `json:"activeBuilders"`
	PendingBuilders   int   `json:"pendingBuilders"`
	ResidentBytes     int64 `json:"residentBytes"`
	ReservedDiskBytes int64 `json:"reservedDiskBytes"`
}

func (s *readSnapshotStore) diagnostics() *ReadSnapshotDiagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := len(s.gate)
	return &ReadSnapshotDiagnostics{Handles: len(s.entries), ActiveBuilders: active, PendingBuilders: max(0, len(s.building)-active), ResidentBytes: s.memory, ReservedDiskBytes: s.disk}
}

func snapshotBinding(kind string, query any) string {
	b, _ := json.Marshal([]any{kind, query})
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func snapshotStale(reason string) error {
	return &SessionOperationError{Code: "stale_cursor", Message: "Read snapshot unavailable (" + reason + "). Reload it.", Retryable: true, ReadReason: reason}
}

func (s *readSnapshotStore) initLocked() {
	if s.entries != nil {
		return
	}
	s.entries = make(map[string]*readSnapshot)
	s.building = make(map[string]*readSnapshotBuild)
	s.gate = make(chan struct{}, 2)
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

// Cancellation belongs to the waiter. A shared build is cancelled only by
// shutdown, so one dismissed UI cannot interrupt another reader's build.
func (s *readSnapshotStore) build(ctx context.Context, binding string, fill func(context.Context, *readSnapshot) error) (*readSnapshot, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, snapshotStale("restarted")
	}
	s.initLocked()
	job := s.building[binding]
	if job == nil {
		if len(s.building) >= 64 {
			s.mu.Unlock()
			return nil, fmt.Errorf("read snapshot resource limit: too many builds")
		}
		job = &readSnapshotBuild{done: make(chan struct{})}
		s.building[binding] = job
		s.buildWG.Add(1)
		go func() {
			defer s.buildWG.Done()
			select {
			case s.gate <- struct{}{}:
				defer func() { <-s.gate }()
			case <-s.ctx.Done():
				job.err = s.ctx.Err()
			}
			var snap *readSnapshot
			if job.err == nil {
				// Reclaim before allocating: expired disk reservations must not
				// prevent the build that would otherwise reclaim them.
				s.pruneExpired()
				var id [24]byte
				_, job.err = rand.Read(id[:])
				snap = &readSnapshot{id: hex.EncodeToString(id[:]), binding: binding, created: time.Now(), used: time.Now(), leases: 1}
				if job.err == nil {
					job.err = fill(s.ctx, snap)
				}
				if job.err == nil && snap.validate != nil {
					job.err = snap.validate()
				}
			}
			s.mu.Lock()
			var victims []*readSnapshot
			if job.err == nil && !s.closed {
				for id, old := range s.entries {
					if time.Since(old.used) >= readSnapshotIdle || time.Since(old.created) >= readSnapshotLife {
						delete(s.entries, id)
						victims = append(victims, old)
					}
				}
				if len(s.entries) >= 64 {
					var oldest *readSnapshot
					for _, old := range s.entries {
						if oldest == nil || old.used.Before(oldest.used) {
							oldest = old
						}
					}
					delete(s.entries, oldest.id)
					victims = append(victims, oldest)
				}
				s.entries[snap.id] = snap
				job.snapshot = snap
			} else if job.err == nil {
				job.err = snapshotStale("restarted")
			}
			delete(s.building, binding)
			s.mu.Unlock()
			for _, old := range victims {
				s.dispose(old)
			}
			if job.err != nil && snap != nil {
				s.dispose(snap)
			}
			close(job.done)
		}()
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-job.done:
		if job.err != nil {
			return nil, job.err
		}
		// Every waiter receives its own idempotent release handle. Shared build
		// storage remains alive until the last handle (or cache lease) expires.
		var id [24]byte
		if _, err := rand.Read(id[:]); err != nil {
			return nil, err
		}
		s.mu.Lock()
		root := job.snapshot
		if s.closed || root.leases <= 0 {
			s.mu.Unlock()
			return nil, snapshotStale("evicted")
		}
		root.leases++
		handle := &readSnapshot{id: hex.EncodeToString(id[:]), binding: binding, created: root.created, used: time.Now(), data: root}
		var victim *readSnapshot
		if len(s.entries) >= 64 {
			for _, old := range s.entries {
				if victim == nil || old.used.Before(victim.used) {
					victim = old
				}
			}
			delete(s.entries, victim.id)
		}
		s.entries[handle.id] = handle
		s.mu.Unlock()
		if victim != nil {
			s.dispose(victim)
		}
		return handle, nil
	}
}

func (s *readSnapshotStore) pruneExpired() {
	s.mu.Lock()
	var victims []*readSnapshot
	for id, snap := range s.entries {
		if time.Since(snap.used) >= readSnapshotIdle || time.Since(snap.created) >= readSnapshotLife {
			delete(s.entries, id)
			victims = append(victims, snap)
		}
	}
	s.mu.Unlock()
	for _, snap := range victims {
		s.dispose(snap)
	}
}

// Append is builder-only. Encoded rows are accounted before storage, including
// unpublished builds. A spill reserves its maximum physical SQLite size.
func (s *readSnapshotStore) append(ctx context.Context, snap *readSnapshot, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	cost := int64(len(b) + 32)
	if snap.size+cost > readSnapshotMax {
		return fmt.Errorf("read snapshot resource limit: result exceeds 64 MiB")
	}
	if snap.db == nil {
		s.mu.Lock()
		resident := s.memory+cost <= readSnapshotMemory && snap.size+cost <= 512<<10
		if resident {
			s.memory += cost
			snap.memory += cost
		}
		s.mu.Unlock()
		if resident {
			snap.rows = append(snap.rows, b)
			snap.size += cost
			snap.count++
			return nil
		}
		s.mu.Lock()
		if s.disk+readSnapshotMax > readSnapshotDisk {
			s.mu.Unlock()
			return fmt.Errorf("read snapshot resource limit: temporary storage exhausted")
		}
		s.disk += readSnapshotMax
		snap.disk = readSnapshotMax
		s.mu.Unlock()
		snap.dir, err = os.MkdirTemp("", "reasonix-read-")
		if err != nil {
			return err
		}
		snap.db, err = sql.Open("sqlite", filepath.Join(snap.dir, "rows.sqlite"))
		if err != nil {
			return err
		}
		snap.db.SetMaxOpenConns(1)
		if _, err = snap.db.ExecContext(ctx, `PRAGMA journal_mode=OFF; PRAGMA max_page_count=16384; CREATE TABLE rows (n INTEGER PRIMARY KEY, body BLOB NOT NULL)`); err != nil {
			return err
		}
		for n, row := range snap.rows {
			if _, err = snap.db.ExecContext(ctx, `INSERT INTO rows VALUES (?,?)`, n, row); err != nil {
				return err
			}
		}
		snap.rows = nil
		s.mu.Lock()
		s.memory -= snap.memory - snap.overhead
		snap.memory = snap.overhead
		s.mu.Unlock()
	}
	if _, err = snap.db.ExecContext(ctx, `INSERT INTO rows VALUES (?,?)`, snap.count, b); err != nil {
		return err
	}
	snap.size += cost
	snap.count++
	return nil
}

// Retained dependency metadata cannot spill, so account for it separately from
// encoded rows. Charges are conservative and remain resident after a spill.
func (s *readSnapshotStore) reserve(snap *readSnapshot, bytes int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memory+bytes > readSnapshotMemory || snap.size+bytes > readSnapshotMax {
		return fmt.Errorf("read snapshot resource limit: dependency metadata exhausted")
	}
	s.memory += bytes
	snap.memory += bytes
	snap.overhead += bytes
	snap.size += bytes
	return nil
}

func (s *readSnapshotStore) page(ctx context.Context, binding, cursor string, first *readSnapshot, limit int, consume func([]byte) error) (string, string, int64, json.RawMessage, error) {
	offset := 0
	id := ""
	if first != nil {
		id = first.id
	}
	if cursor != "" {
		var c readSnapshotCursor
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(b, &c) != nil || c.Version != 1 || c.ID == "" || c.Offset < 0 {
			return "", "", 0, nil, snapshotStale("invalid_cursor")
		}
		if c.Binding != binding {
			return "", "", 0, nil, snapshotStale("query_mismatch")
		}
		id, offset = c.ID, c.Offset
	}
	s.mu.Lock()
	snap := s.entries[id]
	if snap == nil {
		s.mu.Unlock()
		return "", "", 0, nil, snapshotStale("expired_or_evicted")
	}
	if time.Since(snap.used) >= readSnapshotIdle || time.Since(snap.created) >= readSnapshotLife {
		delete(s.entries, id)
		s.mu.Unlock()
		s.dispose(snap)
		return "", "", 0, nil, snapshotStale("expired")
	}
	snap.used = time.Now()
	s.mu.Unlock()
	if snap.data != nil {
		snap = snap.data
	}
	// Per-snapshot lock is a read lease. Disposal cannot close a live DB read.
	snap.mu.Lock()
	defer snap.mu.Unlock()
	if snap.released {
		return "", "", 0, nil, snapshotStale("evicted")
	}
	if snap.binding != binding || offset > snap.count {
		return "", "", 0, nil, snapshotStale("invalid_cursor")
	}
	if snap.validate != nil {
		if err := snap.validate(); err != nil {
			return "", "", 0, nil, err
		}
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	end := min(offset+limit, snap.count)
	if snap.db == nil {
		for _, row := range snap.rows[offset:end] {
			if err := consume(row); err != nil {
				return "", "", 0, nil, err
			}
		}
	} else {
		rows, err := snap.db.QueryContext(ctx, `SELECT body FROM rows WHERE n>=? AND n<? ORDER BY n`, offset, end)
		if err != nil {
			return "", "", 0, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var b []byte
			if err := rows.Scan(&b); err != nil {
				return "", "", 0, nil, err
			}
			if err := consume(b); err != nil {
				return "", "", 0, nil, err
			}
		}
		if err := rows.Err(); err != nil {
			return "", "", 0, nil, err
		}
	}
	next := ""
	if end < snap.count {
		b, _ := json.Marshal(readSnapshotCursor{1, id, binding, end})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	return next, id, snap.created.Add(readSnapshotLife).UnixMilli(), snap.metadata, nil
}

func (s *readSnapshotStore) dispose(snap *readSnapshot) {
	s.mu.Lock()
	if snap.removed {
		s.mu.Unlock()
		return
	}
	snap.removed = true
	if snap.data != nil {
		snap = snap.data
	}
	snap.leases--
	if snap.leases > 0 {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	snap.mu.Lock()
	defer snap.mu.Unlock()
	if snap.released {
		return
	}
	snap.released = true
	if snap.db != nil {
		_ = snap.db.Close()
	}
	if snap.dir != "" {
		_ = os.RemoveAll(snap.dir)
	}
	snap.rows = nil
	snap.metadata, snap.validate = nil, nil
	s.mu.Lock()
	s.memory -= snap.memory
	s.disk -= snap.disk
	s.mu.Unlock()
}

// walk is builder-only and permits streaming conversion of captured candidates
// after the source read transaction has closed.
func (snap *readSnapshot) walk(ctx context.Context, consume func([]byte) error) error {
	if snap.db == nil {
		for _, row := range snap.rows {
			if err := consume(row); err != nil {
				return err
			}
		}
		return nil
	}
	rows, err := snap.db.QueryContext(ctx, `SELECT body FROM rows ORDER BY n`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row []byte
		if err := rows.Scan(&row); err != nil {
			return err
		}
		if err := consume(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (a *App) ReleaseReadSnapshot(id string) {
	s := &a.desktopSessions.readSnapshots
	s.mu.Lock()
	snap := s.entries[id]
	delete(s.entries, id)
	var cached *readSnapshot
	if snap != nil && snap.data != nil {
		cached = s.entries[snap.data.id]
		delete(s.entries, snap.data.id)
	}
	s.mu.Unlock()
	if cached != nil {
		s.dispose(cached)
	}
	if snap != nil {
		s.dispose(snap)
	}
}

func (s *readSnapshotStore) close() {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	entries := s.entries
	s.entries = nil
	jobs := make([]*readSnapshotBuild, 0, len(s.building))
	for _, job := range s.building {
		jobs = append(jobs, job)
	}
	s.mu.Unlock()
	s.buildWG.Wait()
	for _, job := range jobs {
		<-job.done
	}
	for _, snap := range entries {
		s.dispose(snap)
	}
}
