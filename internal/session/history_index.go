package session

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/projectiondb"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontent"
)

const (
	historyIndexVersion     = 1
	HistoryPageDefaultLimit = 100
	HistoryPageMaxLimit     = 500
	HistoryPageMaxBytes     = 2 << 20
)

// PersistentMessage is the storage/query representation of a message. It is
// deliberately separate from provider.Message: provider DTOs are materialized
// only at model or compatibility boundaries.
type PersistentMessage struct {
	MessageID     string              `json:"messageId"`
	Position      int64               `json:"position"`
	Version       int                 `json:"version"`
	Role          string              `json:"role"`
	Preview       string              `json:"preview,omitempty"`
	EventSequence uint64              `json:"eventSequence"`
	Inline        json.RawMessage     `json:"inline,omitempty"`
	ContentRef    *sessioncontent.Ref `json:"contentRef,omitempty"`
}

type MessageHistoryPage struct {
	Messages         []PersistentMessage `json:"messages"`
	SnapshotSequence uint64              `json:"snapshotSequence"`
	NextCursor       string              `json:"nextCursor,omitempty"`
	HasMore          bool                `json:"hasMore"`
}

type historyCursor struct {
	SessionID        string `json:"sessionId"`
	StorageRevision  int    `json:"storageRevision"`
	SnapshotSequence uint64 `json:"snapshotSequence"`
	AfterPosition    int64  `json:"afterPosition"`
	Projection       int    `json:"projection"`
}

type historyBuildState struct {
	nextPosition int64
	positions    map[string]int64
	versions     map[string]int
}

func historyIndexPath(root, sessionID string) string {
	return filepath.Join(root, ".query-cache", filepath.Base(sessionID), "history-v1.sqlite")
}

var historyMigrations = []projectiondb.Migration{{Version: historyIndexVersion, Apply: func(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE transactions (commit_id TEXT PRIMARY KEY, first_sequence INTEGER NOT NULL, last_sequence INTEGER NOT NULL, operation_id TEXT NOT NULL UNIQUE, operation_hash TEXT NOT NULL, turn_id TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE events (sequence INTEGER PRIMARY KEY, commit_id TEXT NOT NULL, event_id TEXT NOT NULL UNIQUE, kind TEXT NOT NULL, payload_digest TEXT NOT NULL DEFAULT '', payload_bytes INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE messages (message_id TEXT NOT NULL, version INTEGER NOT NULL, position INTEGER NOT NULL, event_sequence INTEGER NOT NULL, valid_to INTEGER NOT NULL DEFAULT 0, role TEXT NOT NULL, preview TEXT NOT NULL, inline BLOB, content_digest TEXT NOT NULL DEFAULT '', content_bytes INTEGER NOT NULL DEFAULT 0, content_index_digest TEXT NOT NULL DEFAULT '', current INTEGER NOT NULL, PRIMARY KEY(message_id, version))`,
		`CREATE UNIQUE INDEX messages_current_position ON messages(position) WHERE current=1`,
		`CREATE INDEX messages_current_id ON messages(message_id) WHERE current=1`,
		`CREATE TABLE content_refs (digest TEXT NOT NULL, bytes INTEGER NOT NULL, index_digest TEXT NOT NULL DEFAULT '', PRIMARY KEY(digest, bytes, index_digest))`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}}}

func (q *Query) HistoryPage(ctx context.Context, ref SessionRef, cursor string, limit int) (MessageHistoryPage, error) {
	if q == nil {
		return MessageHistoryPage{}, errors.New("session: nil query")
	}
	if err := ref.validate(q.hostID); err != nil {
		return MessageHistoryPage{}, err
	}
	filesystem, ok := q.persistence.(*FilesystemPersistence)
	if !ok {
		return MessageHistoryPage{}, errors.New("session: history index requires filesystem persistence")
	}
	if limit <= 0 {
		limit = HistoryPageDefaultLimit
	}
	limit = min(limit, HistoryPageMaxLimit)
	path := historyIndexPath(filesystem.Root, ref.SessionID)
	q.rebuildMu.Lock()
	err := ensureHistoryIndex(ctx, filesystem, ref.SessionID, path)
	q.rebuildMu.Unlock()
	if err != nil {
		return MessageHistoryPage{}, err
	}
	handle, err := projectiondb.Open(ctx, projectiondb.OpenOptions{Path: path, Migrations: historyMigrations, RequireDisk: true, MaxOpenConns: 1})
	if err != nil {
		return MessageHistoryPage{}, err
	}
	defer handle.DB.Close()
	var snapshot uint64
	if err := scanMetadataUint(handle.DB.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key='durable_sequence'`), &snapshot); err != nil {
		return MessageHistoryPage{}, err
	}
	after := int64(0)
	if cursor != "" {
		parsed, err := decodeHistoryCursor(cursor)
		if err != nil {
			return MessageHistoryPage{}, err
		}
		if parsed.SessionID != ref.SessionID || parsed.StorageRevision != StorageRevision || parsed.Projection != historyIndexVersion || parsed.SnapshotSequence > snapshot {
			return MessageHistoryPage{}, errors.New("session: history cursor no longer matches this snapshot")
		}
		snapshot = parsed.SnapshotSequence
		after = parsed.AfterPosition
	}
	rows, err := handle.DB.QueryContext(ctx, `SELECT message_id,position,version,role,preview,event_sequence,inline,content_digest,content_bytes,content_index_digest FROM messages WHERE position>? AND event_sequence<=? AND (valid_to=0 OR valid_to>?) ORDER BY position LIMIT ?`, after, snapshot, snapshot, limit+1)
	if err != nil {
		return MessageHistoryPage{}, err
	}
	defer rows.Close()
	page := MessageHistoryPage{Messages: []PersistentMessage{}, SnapshotSequence: snapshot}
	encodedBytes := 0
	for rows.Next() {
		var message PersistentMessage
		var inline []byte
		var digest, indexDigest string
		var contentBytes int64
		if err := rows.Scan(&message.MessageID, &message.Position, &message.Version, &message.Role, &message.Preview, &message.EventSequence, &inline, &digest, &contentBytes, &indexDigest); err != nil {
			return MessageHistoryPage{}, err
		}
		if len(page.Messages) == limit {
			page.HasMore = true
			break
		}
		message.Inline = append(json.RawMessage(nil), inline...)
		if digest != "" {
			message.ContentRef = &sessioncontent.Ref{Digest: digest, Bytes: contentBytes, IndexDigest: indexDigest, IntegrityBlock: sessioncontent.IntegrityBlockBytes, MediaType: "application/json"}
		}
		encoded, _ := json.Marshal(message)
		if len(page.Messages) > 0 && encodedBytes+len(encoded) > HistoryPageMaxBytes {
			page.HasMore = true
			break
		}
		encodedBytes += len(encoded)
		page.Messages = append(page.Messages, message)
	}
	if err := rows.Err(); err != nil {
		return MessageHistoryPage{}, err
	}
	if page.HasMore && len(page.Messages) > 0 {
		last := page.Messages[len(page.Messages)-1]
		page.NextCursor, err = encodeHistoryCursor(historyCursor{SessionID: ref.SessionID, StorageRevision: StorageRevision, SnapshotSequence: snapshot, AfterPosition: last.Position, Projection: historyIndexVersion})
		if err != nil {
			return MessageHistoryPage{}, err
		}
	}
	return page, nil
}

func (q *Query) ReadContent(ctx context.Context, ref SessionRef, contentRef sessioncontent.Ref, offset, length int64) ([]byte, error) {
	if err := ref.validate(q.hostID); err != nil {
		return nil, err
	}
	filesystem, ok := q.persistence.(*FilesystemPersistence)
	if !ok {
		return nil, errors.New("session: content reads require filesystem persistence")
	}
	path := historyIndexPath(filesystem.Root, ref.SessionID)
	q.rebuildMu.Lock()
	err := ensureHistoryIndex(ctx, filesystem, ref.SessionID, path)
	q.rebuildMu.Unlock()
	if err != nil {
		return nil, err
	}
	handle, err := projectiondb.Open(ctx, projectiondb.OpenOptions{Path: path, Migrations: historyMigrations, RequireDisk: true, MaxOpenConns: 1})
	if err != nil {
		return nil, err
	}
	defer handle.DB.Close()
	var allowed int
	if err := handle.DB.QueryRowContext(ctx, `SELECT 1 FROM content_refs WHERE digest=? AND bytes=? AND index_digest=?`, contentRef.Digest, contentRef.Bytes, contentRef.IndexDigest).Scan(&allowed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("session: content reference is not authorized for this session")
		}
		return nil, err
	}
	return sessioncontent.New(filepath.Join(filesystem.Root, ".content-v1")).ReadRange(ctx, contentRef, offset, length)
}

func ensureHistoryIndex(ctx context.Context, persistence *FilesystemPersistence, sessionID, path string) error {
	dir := filepath.Join(persistence.Root, sessionID)
	revision, err := revisionOfLog(dir)
	if err != nil {
		return err
	}
	if historyIndexCurrent(ctx, path, sessionID, revision) {
		return nil
	}
	return rebuildHistoryIndex(ctx, dir, path, sessionID, revision)
}

func historyIndexCurrent(ctx context.Context, path, sessionID string, revision logRevision) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	handle, err := projectiondb.Open(ctx, projectiondb.OpenOptions{Path: path, Migrations: historyMigrations, RequireDisk: true, MaxOpenConns: 1})
	if err != nil {
		return false
	}
	defer handle.DB.Close()
	values := map[string]string{}
	rows, err := handle.DB.QueryContext(ctx, `SELECT key,value FROM metadata WHERE key IN ('session_id','log_size','log_mtime_ns','storage_revision')`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) != nil {
			return false
		}
		values[key] = value
	}
	return values["session_id"] == sessionID && values["log_size"] == fmt.Sprint(revision.Size) && values["log_mtime_ns"] == fmt.Sprint(revision.ModTimeNS) && values["storage_revision"] == fmt.Sprint(StorageRevision)
}

func rebuildHistoryIndex(ctx context.Context, dir, path, sessionID string, revision logRevision) error {
	manifest, err := readManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	log, err := os.Open(logPathForManifest(dir, manifest))
	if err != nil {
		return err
	}
	defer log.Close()
	content := contentStoreForSessionDir(dir)
	return projectiondb.Rebuild(ctx, projectiondb.OpenOptions{Path: path, Migrations: historyMigrations, RequireDisk: true, MaxOpenConns: 1}, func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		state := historyBuildState{positions: map[string]int64{}, versions: map[string]int{}}
		var durable uint64
		var buildErr error
		err = scanV4CommitFileRefs(ctx, log, 0, 1, content, nil, func(_ int64, commit Commit) bool {
			if _, err := tx.ExecContext(ctx, `INSERT INTO transactions(commit_id,first_sequence,last_sequence,operation_id,operation_hash,turn_id,created_at) VALUES(?,?,?,?,?,?,?)`, commit.ID, commit.FirstSequence, commit.LastSequence(), commit.OperationID, commit.OperationHash, commit.TurnID, commit.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")); err != nil {
				buildErr = err
				return false
			}
			for _, event := range commit.Events {
				digest := ""
				var bytes int64
				if event.PayloadRef != nil {
					digest, bytes = event.PayloadRef.Digest, event.PayloadRef.Bytes
					if err := insertContentRef(ctx, tx, *event.PayloadRef); err != nil {
						buildErr = err
						return false
					}
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO events(sequence,commit_id,event_id,kind,payload_digest,payload_bytes) VALUES(?,?,?,?,?,?)`, event.Sequence, commit.ID, event.ID, event.Kind, digest, bytes); err != nil {
					buildErr = err
					return false
				}
				if err := indexMessageEvent(ctx, tx, content, &state, event); err != nil {
					buildErr = err
					return false
				}
				durable = event.Sequence
			}
			return true
		})
		if err != nil {
			return err
		}
		if buildErr != nil {
			return buildErr
		}
		metadata := map[string]string{"session_id": sessionID, "log_size": fmt.Sprint(revision.Size), "log_mtime_ns": fmt.Sprint(revision.ModTimeNS), "storage_revision": fmt.Sprint(StorageRevision), "durable_sequence": fmt.Sprint(durable)}
		for key, value := range metadata {
			if _, err := tx.ExecContext(ctx, `INSERT INTO metadata(key,value) VALUES(?,?)`, key, value); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func indexMessageEvent(ctx context.Context, tx *sql.Tx, content *sessioncontent.Store, state *historyBuildState, event Event) error {
	if event.Kind != "message/complete" && event.Kind != "message/upsert" && event.Kind != "history/replace" && event.Kind != "legacy/import" {
		return nil
	}
	payload := event.Payload
	if event.PayloadRef != nil {
		var err error
		payload, err = resolveContentPayload(ctx, content, *event.PayloadRef)
		if err != nil {
			return err
		}
	}
	switch event.Kind {
	case "message/complete", "message/upsert":
		var body struct {
			Message *provider.Message `json:"message"`
		}
		if err := strictPayload(payload, &body); err != nil || body.Message == nil {
			return damagedPayload(event, err)
		}
		return indexOneMessage(ctx, tx, content, state, *body.Message, event.Sequence, event.Kind == "message/upsert")
	case "history/replace":
		var body struct {
			Messages []provider.Message `json:"messages"`
		}
		if err := strictPayload(payload, &body); err != nil || body.Messages == nil {
			return damagedPayload(event, err)
		}
		return replaceIndexedMessages(ctx, tx, content, state, body.Messages, event.Sequence)
	case "legacy/import":
		var body struct {
			Messages []provider.Message `json:"messages"`
		}
		if err := strictPayload(payload, &body); err != nil || body.Messages == nil {
			return damagedPayload(event, err)
		}
		return replaceIndexedMessages(ctx, tx, content, state, body.Messages, event.Sequence)
	}
	return nil
}

func replaceIndexedMessages(ctx context.Context, tx *sql.Tx, content *sessioncontent.Store, state *historyBuildState, messages []provider.Message, sequence uint64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET current=0,valid_to=? WHERE current=1`, sequence); err != nil {
		return err
	}
	state.nextPosition = 0
	state.positions = map[string]int64{}
	state.versions = map[string]int{}
	for _, message := range messages {
		if err := indexOneMessage(ctx, tx, content, state, message, sequence, false); err != nil {
			return err
		}
	}
	return nil
}

func indexOneMessage(ctx context.Context, tx *sql.Tx, content *sessioncontent.Store, state *historyBuildState, message provider.Message, sequence uint64, upsert bool) error {
	id := strings.TrimSpace(message.ID)
	if id == "" {
		return errors.New("session: indexed message has no stable id")
	}
	position, exists := state.positions[id]
	if !exists {
		state.nextPosition++
		position = state.nextPosition
		state.positions[id] = position
	} else if !upsert {
		return fmt.Errorf("session: duplicate indexed message id %q", id)
	}
	version := state.versions[id] + 1
	state.versions[id] = version
	if exists {
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET current=0,valid_to=? WHERE message_id=? AND current=1`, sequence, id); err != nil {
			return err
		}
	}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	var inline []byte
	var ref sessioncontent.Ref
	if len(body) > v4InlinePayloadBytes {
		ref, err = content.Put(ctx, bytes.NewReader(body), sessioncontent.Metadata{MediaType: "application/json"})
		if err != nil {
			return err
		}
		if err := insertContentRef(ctx, tx, ref); err != nil {
			return err
		}
	} else {
		inline = body
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO messages(message_id,version,position,event_sequence,valid_to,role,preview,inline,content_digest,content_bytes,content_index_digest,current) VALUES(?,?,?,?,0,?,?,?,?,?,?,1)`, id, version, position, sequence, string(message.Role), messagePreview(message), inline, ref.Digest, ref.Bytes, ref.IndexDigest)
	return err
}

func insertContentRef(ctx context.Context, tx *sql.Tx, ref sessioncontent.Ref) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO content_refs(digest,bytes,index_digest) VALUES(?,?,?)`, ref.Digest, ref.Bytes, ref.IndexDigest)
	return err
}

func messagePreview(message provider.Message) string {
	preview := strings.TrimSpace(message.Content)
	if preview == "" {
		preview = strings.TrimSpace(message.RawContent)
	}
	runes := []rune(preview)
	if len(runes) > 240 {
		preview = string(runes[:240])
	}
	return preview
}

func encodeHistoryCursor(cursor historyCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeHistoryCursor(value string) (historyCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return historyCursor{}, errors.New("session: invalid history cursor")
	}
	var cursor historyCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return historyCursor{}, errors.New("session: invalid history cursor")
	}
	return cursor, nil
}

func scanMetadataUint(row *sql.Row, target *uint64) error {
	var value string
	if err := row.Scan(&value); err != nil {
		return err
	}
	_, err := fmt.Sscan(value, target)
	return err
}
