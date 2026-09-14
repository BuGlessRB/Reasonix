package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

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
	projection := runtime.Session().Snapshot().Projection
	if len(projection.Messages) != 0 || len(projection.ModelMessages) != 3 {
		t.Fatalf("service runtime retained durable UI bodies: messages=%d model=%d", len(projection.Messages), len(projection.ModelMessages))
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
