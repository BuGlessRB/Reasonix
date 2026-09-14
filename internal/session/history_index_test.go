package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/projectiondb"
	"reasonix/internal/provider"
)

func TestHistoryPageKeepsSnapshotAndAuthorizesReferencedContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions-v4")
	service, err := NewService("local", NewFilesystemPersistence(root))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := service.Create(t.Context(), CreateOptions{SessionID: "paged"})
	if err != nil {
		t.Fatal(err)
	}
	appendMessage := func(id, content string) {
		payload, err := json.Marshal(map[string]any{"message": provider.Message{ID: id, Role: provider.RoleUser, Content: content}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Session().Append(t.Context(), Batch{OperationID: "message-" + id, Events: []Event{{Kind: "message/complete", Payload: payload}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Session().Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	appendMessage("one", "first")
	appendMessage("two", strings.Repeat("large", 20000))
	appendMessage("three", "third")
	runtime.Session().mu.Lock()
	residentMessages := len(runtime.Session().projection.Messages)
	residentModel := len(runtime.Session().projection.ModelMessages)
	runtime.Session().mu.Unlock()
	if residentMessages != 0 || residentModel != 3 {
		t.Fatalf("service runtime retained durable UI bodies: messages=%d model=%d", residentMessages, residentModel)
	}
	ref := runtime.Ref()
	first, err := service.Query().HistoryPage(t.Context(), ref, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 1 || first.Messages[0].MessageID != "three" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	appendMessage("four", "must not enter the fixed snapshot")
	second, err := service.Query().HistoryPage(t.Context(), ref, first.NextCursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) != 2 || second.Messages[0].MessageID != "one" || second.Messages[1].MessageID != "two" {
		t.Fatalf("fixed snapshot second page = %+v", second)
	}
	large := second.Messages[1]
	if large.ContentRef == nil || len(large.Inline) != 0 {
		t.Fatalf("large message was not referenced: %+v", large)
	}
	chunk, err := service.Query().ReadContent(t.Context(), ref, *large.ContentRef, 0, min(64, large.ContentRef.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	var decoded provider.Message
	full, err := service.Query().ReadContent(t.Context(), ref, *large.ContentRef, 0, large.ContentRef.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk) == 0 || json.Unmarshal(full, &decoded) != nil || decoded.ID != "two" {
		t.Fatalf("resolved content prefix=%q id=%q", chunk, decoded.ID)
	}
	foreign := *large.ContentRef
	foreign.Digest = strings.Repeat("0", len(foreign.Digest))
	if _, err := service.Query().ReadContent(t.Context(), ref, foreign, 0, 1); err == nil {
		t.Fatal("content hash without a session reference was authorized")
	}
}

func TestSearchHistoryUsesStableSnapshotAndOpaqueQueryCursor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions-v4")
	service, err := NewService("local", NewFilesystemPersistence(root))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := service.Create(t.Context(), CreateOptions{SessionID: "search"})
	if err != nil {
		t.Fatal(err)
	}
	appendMessage := func(id, content string) {
		payload, _ := json.Marshal(map[string]any{"message": provider.Message{ID: id, Role: provider.RoleUser, Content: content}})
		if _, err := runtime.Session().Append(t.Context(), Batch{OperationID: id, Events: []Event{{Kind: "message/complete", Payload: payload}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Session().Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	appendMessage("one", "first needle")
	appendMessage("two", "second needle")
	appendMessage("three", "unrelated")
	first, err := service.Query().SearchHistory(t.Context(), runtime.Ref(), "needle", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Hits) != 1 || first.Hits[0].MessageID != "two" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first search = %+v", first)
	}
	appendMessage("four", "new needle outside snapshot")
	second, err := service.Query().SearchHistory(t.Context(), runtime.Ref(), "needle", first.NextCursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Hits) != 1 || second.Hits[0].MessageID != "one" {
		t.Fatalf("second search = %+v", second)
	}
	if _, err := service.Query().SearchHistory(t.Context(), runtime.Ref(), "different", first.NextCursor, 10); err == nil {
		t.Fatal("search cursor was accepted for another query")
	}
}

func TestHistoryIndexHasSnapshotPositionIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.sqlite")
	handle, err := projectiondb.Open(t.Context(), projectiondb.OpenOptions{Path: path, Migrations: historyMigrations, RequireDisk: true, MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.DB.Close()
	rows, err := handle.DB.QueryContext(context.Background(), `PRAGMA index_list(messages)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		found = found || name == "messages_snapshot_position"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("snapshot-position query index is missing")
	}
}
