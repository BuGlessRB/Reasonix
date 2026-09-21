package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/history"
	"reasonix/internal/historycatalog"
	"reasonix/internal/provider"
	"reasonix/internal/retrieval"
	"reasonix/internal/sessioncatalog"
)

func snapshotReadError(err error) (*ReadError, bool) {
	if err == nil {
		return nil, false
	}
	var op *SessionOperationError
	if errors.As(err, &op) && op.Code == "stale_cursor" {
		return &ReadError{Code: "stale_cursor", Reason: op.ReadReason, Message: err.Error()}, true
	}
	return &ReadError{Code: "read_failed", Reason: "read_failed", Message: err.Error()}, false
}

func (a *App) ListHistorySessions(req HistorySessionPageRequest) HistorySessionPage {
	out := HistorySessionPage{Items: []SessionMeta{}}
	key := req
	key.Cursor, key.Limit = "", 0
	active := a.activeSessionPath(a.activeSessionDir())
	binding := snapshotBinding("history-sessions", []any{key, active})
	store := &a.desktopSessions.readSnapshots
	var first *readSnapshot
	var err error
	if req.Cursor == "" {
		first, err = store.build(a.bootContext(), binding, func(ctx context.Context, snap *readSnapshot) error {
			meta := HistorySessionPage{Partial: true}
			catalog := a.sessionCatalog.Load()
			if catalog == nil {
				snap.metadata, _ = json.Marshal(meta)
				return nil
			}
			_, overlays := a.catalogRuntimeOverlays()
			fence, err := a.newReadSourceFence(store, snap)
			if err != nil {
				return err
			}
			fence.metadataOnly = true
			err = catalog.WithReadView(ctx, func(view context.Context) error {
				cursor := ""
				for {
					page, err := catalog.ListSessions(view, sessioncatalog.SessionPageRequest{Scope: req.Scope, WorkspaceRoot: req.WorkspaceRoot, Query: req.Query, TimeFilter: req.TimeFilter, Cursor: cursor, Limit: 200})
					if err != nil {
						return err
					}
					if page.StaleCursor {
						return snapshotStale("lifecycle_changed")
					}
					meta.Revision = page.Revision
					for _, row := range page.Items {
						overlay := overlays[sessionRuntimeKey(row.Path)]
						if !historyStatusMatches(req.Status, overlay.open, row.Path == active) {
							continue
						}
						if err := fence.add(view, row.Path); err != nil {
							return err
						}
						if err := store.append(ctx, snap, sessionMetaFromCatalog(row, row.Path == active, overlay.open)); err != nil {
							return err
						}
					}
					if page.NextCursor == "" {
						return nil
					}
					if page.NextCursor == cursor {
						return errors.New("history catalog cursor did not advance")
					}
					cursor = page.NextCursor
				}
			})
			if err != nil {
				return err
			}
			status := catalog.Status()
			snap.validate = fence.freeze()
			meta.Partial = status.State != sessioncatalog.StateReady || status.Indexed < status.Total
			snap.metadata, err = json.Marshal(meta)
			return err
		})
	}
	if err == nil {
		var meta json.RawMessage
		out.NextCursor, out.SnapshotID, out.SnapshotExpiresAt, meta, err = store.page(a.bootContext(), binding, req.Cursor, first, req.Limit, func(b []byte) error {
			var row SessionMeta
			if err := json.Unmarshal(b, &row); err != nil {
				return err
			}
			out.Items = append(out.Items, row)
			return nil
		})
		if err == nil {
			var frozen HistorySessionPage
			err = json.Unmarshal(meta, &frozen)
			out.Revision, out.Partial = frozen.Revision, frozen.Partial
		}
	}
	if err != nil {
		out = HistorySessionPage{Items: []SessionMeta{}}
		out.ReadError, out.StaleCursor = snapshotReadError(err)
	}
	return out
}

func (a *App) SearchHistoryContent(req HistorySearchRequest) HistorySearchPage {
	return a.searchHistorySnapshot(req, "")
}

func (a *App) searchHistorySnapshot(req HistorySearchRequest, targetPath string) HistorySearchPage {
	out := HistorySearchPage{Items: []HistorySearchHit{}}
	req.Query = strings.TrimSpace(req.Query)
	req.Kinds = append([]string(nil), req.Kinds...)
	if len(req.Kinds) == 0 {
		req.Kinds = []string{"user_text", "assistant_text", "tool_input", "tool_error"}
	}
	sort.Strings(req.Kinds)
	key := req
	key.Cursor, key.Limit = "", 0
	active := a.activeSessionPath(a.activeSessionDir())
	activeBinding := active
	if targetPath != "" {
		// Exact-target reads belong to the selected source, not foreground focus.
		activeBinding = ""
	}
	binding := snapshotBinding("history-search", []any{key, targetPath, activeBinding})
	store := &a.desktopSessions.readSnapshots
	var first *readSnapshot
	var err error
	if req.Cursor == "" {
		first, err = store.build(a.bootContext(), binding, func(ctx context.Context, snap *readSnapshot) error {
			status := a.GetHistoryIndexStatus()
			meta := HistorySearchPage{Status: status, Revision: status.Revision, Partial: status.State != "ready" || status.Pending > 0}
			catalog := history.SharedCatalog()
			if catalog == nil || req.Query == "" {
				snap.metadata, _ = json.Marshal(meta)
				return nil
			}
			candidates := &readSnapshot{}
			defer store.dispose(candidates)
			roots := historySearchRootFilter(a, req)
			if targetPath != "" {
				roots = nil
			}
			if err := catalog.CaptureSearch(ctx, historycatalog.SearchRequest{Query: req.Query, Scope: req.Scope, WorkspaceRoot: req.WorkspaceRoot, SessionPath: targetPath, Kinds: req.Kinds, ToolName: req.ToolName, Roots: roots}, func(row historycatalog.Candidate) error { return store.append(ctx, candidates, row) }); err != nil {
				return err
			}
			_, overlays := a.catalogRuntimeOverlays()
			terms, err := retrieval.QueryTerms(req.Query)
			if err != nil {
				return err
			}
			// Keep at most one source transcript resident. Ranking may interleave
			// sources; rereading is preferable to an unbounded transcript cache.
			var messages []provider.Message
			var loadedPath, loadedDigest string
			var covered, intact bool
			cutoffTime := time.Now()
			fence, err := a.newReadSourceFence(store, snap)
			if err != nil {
				return err
			}
			err = candidates.walk(ctx, func(b []byte) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				var row historycatalog.Candidate
				if err := json.Unmarshal(b, &row); err != nil {
					return err
				}
				overlay := overlays[sessionRuntimeKey(row.SessionPath)]
				if !historyStatusMatches(req.Status, overlay.open, row.SessionPath == active) || !historyTimeMatchesAt(row.LastActivityAt, req.TimeFilter, cutoffTime) {
					return nil
				}
				if loadedPath != row.SessionPath {
					loadedPath, covered = row.SessionPath, false
					if sessions := a.sessionCatalog.Load(); sessions != nil {
						record, ok, err := sessions.GetSession(ctx, row.SessionPath)
						if err != nil {
							return err
						}
						covered = ok && record.RecoveryCopy
					}
					if !covered {
						if err := fence.add(ctx, row.SessionPath); err != nil {
							if !errors.Is(err, os.ErrNotExist) {
								return err
							}
							meta.Partial = true
							intact = false
							messages = nil
							return nil
						}
						var state agent.PersistedState
						messages, state, intact, err = agent.LoadSessionDisplayMessages(row.SessionPath)
						if errors.Is(err, os.ErrNotExist) {
							meta.Partial = true
							messages = nil
							intact = false
						} else if err != nil {
							return err
						}
						loadedDigest = state.DigestHex
					}
				}
				if covered {
					return nil
				}
				if !intact || row.ContentDigest == "" || loadedDigest != row.ContentDigest {
					catalog.EnqueueExisting(ctx, row.SessionPath)
					meta.Partial = true
					return nil
				}
				text, ok := desktopHistoryText(messages, row)
				if !ok {
					catalog.EnqueueExisting(ctx, row.SessionPath)
					meta.Partial = true
					return nil
				}
				if err := fence.add(ctx, row.SessionPath); err != nil {
					return err
				}
				hit := HistorySearchHit{SessionPath: row.SessionPath, SessionID: strings.TrimSuffix(filepath.Base(row.SessionPath), filepath.Ext(row.SessionPath)), Source: row.Source, MessageIndex: row.MessageIndex, PartIndex: row.PartIndex, ContentDigest: loadedDigest, Role: row.Role, Kind: row.Kind, ToolName: row.ToolName, Snippet: retrieval.MakeSnippet(text, req.Query, terms, 240), Score: row.Score, SessionTitle: row.SessionTitle, TopicTitle: row.TopicTitle, WorkspaceRoot: row.WorkspaceRoot, LastActivityAt: row.LastActivityAt, Open: overlay.open, Running: overlay.running, Current: row.SessionPath == active}
				return store.append(ctx, snap, hit)
			})
			if err != nil {
				return err
			}
			snap.validate = fence.freeze()
			snap.metadata, err = json.Marshal(meta)
			return err
		})
	}
	if err == nil {
		var meta json.RawMessage
		out.NextCursor, out.SnapshotID, out.SnapshotExpiresAt, meta, err = store.page(a.bootContext(), binding, req.Cursor, first, req.Limit, func(b []byte) error {
			var row HistorySearchHit
			if err := json.Unmarshal(b, &row); err != nil {
				return err
			}
			out.Items = append(out.Items, row)
			return nil
		})
		if err == nil {
			var frozen HistorySearchPage
			err = json.Unmarshal(meta, &frozen)
			out.Revision, out.Partial, out.Status = frozen.Revision, frozen.Partial, frozen.Status
		}
	}
	if err != nil {
		out = HistorySearchPage{Items: []HistorySearchHit{}}
		out.ReadError, out.StaleCursor = snapshotReadError(err)
		out.Status.LastError = err.Error()
	}
	return out
}
