package sessioncatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"reasonix/internal/agent"
)

// seedCatalogSession writes a real transcript plus sidecar, since the memory
// index reads what is on disk rather than what a test handed the writer.
func seedCatalogSession(t *testing.T, dir, name, title string, turns int, activity time.Time, recovered bool) string {
	t.Helper()
	path := filepath.Join(dir, name+".jsonl")
	sess := agent.NewSession("sys")
	for i := range turns {
		sess.Add(agentMessage("user", fmt.Sprintf("%s ask %d", title, i)))
		sess.Add(agentMessage("assistant", fmt.Sprintf("%s answer %d", title, i)))
	}
	if err := sess.Save(path); err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
	meta, _, err := agent.LoadBranchMeta(path)
	if err != nil {
		t.Fatalf("load meta %s: %v", name, err)
	}
	meta.TopicTitle = title
	meta.CustomTitle = title
	meta.UpdatedAt = activity
	meta.Recovered = recovered
	if err := agent.SaveBranchMetaPreserveUpdated(path, meta); err != nil {
		t.Fatalf("save meta %s: %v", name, err)
	}
	if err := os.Chtimes(path, activity, activity); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return path
}

// seedCatalogDirectory fills one directory with a spread of activity times,
// titles and one conflict copy.
func seedCatalogDirectory(t *testing.T, dir string, prefix string, base time.Time) {
	t.Helper()
	for i := range 12 {
		title := fmt.Sprintf("%s topic %d", prefix, i)
		if i%3 == 0 {
			title = fmt.Sprintf("%s alpha %d", prefix, i)
		}
		seedCatalogSession(t, dir, fmt.Sprintf("%s-%02d", prefix, i), title, i%4+1,
			base.Add(-time.Duration(i)*7*time.Hour), false)
	}
	seedCatalogSession(t, dir, prefix+"-conflict", prefix+" topic 0", 2, base.Add(-3*time.Hour), true)
	// Same instant, so ordering has to fall through to the path tie-break —
	// the branch a spread of distinct timestamps never reaches.
	tie := base.Add(-90 * time.Minute)
	for _, suffix := range []string{"tie-c", "tie-a", "tie-b"} {
		seedCatalogSession(t, dir, prefix+"-"+suffix, prefix+" tie", 1, tie, false)
	}
}

// TestMemIndexMatchesCatalogReads runs both projections over the same
// directories and compares what each returns. The memory index is only a
// candidate replacement if it answers identically — ordering, paging cursors
// and all — so this is the gate, not the benchmark.
func TestMemIndexMatchesCatalogReads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	seedCatalogDirectory(t, globalDir, "g", base)
	seedCatalogDirectory(t, projectDir, "p", base)

	targets := []DirectoryTarget{
		{Path: globalDir, Scope: "global"},
		{Path: projectDir, Scope: "project", WorkspaceRoot: "/work/root"},
	}
	now := func() time.Time { return base }
	catalog, err := Open(ctx, Options{Path: filepath.Join(t.TempDir(), "catalog.sqlite"), DisableRepair: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	mem := NewMemIndex(now)
	for _, target := range targets {
		if err := catalog.ReconcileDirectory(ctx, target); err != nil {
			t.Fatalf("reconcile %s: %v", target.Path, err)
		}
		if err := mem.ScanDirectory(target); err != nil {
			t.Fatalf("scan %s: %v", target.Path, err)
		}
	}

	requests := []SessionPageRequest{
		{Scope: "all", Limit: 50},
		{Scope: "all", Limit: 5},
		{Scope: "global", Limit: 50},
		{Scope: "project", WorkspaceRoot: "/work/root", Limit: 50},
		{Scope: "project", WorkspaceRoot: "/other", Limit: 50},
		{Scope: "all", Directory: globalDir, Limit: 50},
		{Scope: "all", Query: "alpha", Limit: 50},
		{Scope: "all", Query: "g topic", Limit: 50},
		{Scope: "all", TimeFilter: "today", Limit: 50},
		{Scope: "all", TimeFilter: "yesterday", Limit: 50},
		{Scope: "all", TimeFilter: "older", Limit: 50},
	}
	// Two identically empty answers compare equal, so pin what the broad query
	// must actually return: 12 sessions per directory, with the conflict copy
	// left out of both, plus three sharing one instant.
	if page, err := mem.ListSessions(SessionPageRequest{Scope: "all", Limit: 50}); err != nil || len(page.Items) != 30 {
		t.Fatalf("mem full page = %d items err=%v, want 30", len(page.Items), err)
	}
	// Each time bucket must be non-empty and the three must partition the set,
	// otherwise comparing them proves only that both sides filter nothing.
	buckets := map[string]int{}
	for _, filter := range []string{"today", "yesterday", "older"} {
		page, err := mem.ListSessions(SessionPageRequest{Scope: "all", TimeFilter: filter, Limit: 50})
		if err != nil || len(page.Items) == 0 {
			t.Fatalf("time filter %q returned %d items err=%v", filter, len(page.Items), err)
		}
		buckets[filter] = len(page.Items)
	}
	if total := buckets["today"] + buckets["yesterday"] + buckets["older"]; total != 30 {
		t.Fatalf("time buckets sum to %d, want the full 30: %v", total, buckets)
	}
	for _, req := range requests {
		want, err := catalog.ListSessions(ctx, req)
		if err != nil {
			t.Fatalf("catalog %+v: %v", req, err)
		}
		got, err := mem.ListSessions(req)
		if err != nil {
			t.Fatalf("mem %+v: %v", req, err)
		}
		comparePages(t, fmt.Sprintf("%+v", req), want, got)
	}

	// Paging is where an ordering difference actually shows: walk both to the
	// end and require the same sessions in the same order across page breaks.
	compareWalk(t, ctx, catalog, mem, SessionPageRequest{Scope: "all", Limit: 5})

	for _, path := range []string{filepath.Join(globalDir, "g-03.jsonl"), filepath.Join(projectDir, "p-07.jsonl")} {
		want, ok, err := catalog.GetSession(ctx, path)
		if err != nil || !ok {
			t.Fatalf("catalog GetSession %s: ok=%v err=%v", path, ok, err)
		}
		got, ok := mem.GetSession(path)
		if !ok {
			t.Fatalf("mem GetSession %s: not found", path)
		}
		if want.Path != got.Path || want.Turns != got.Turns || want.TopicTitle != got.TopicTitle {
			t.Errorf("GetSession %s: catalog=%+v mem=%+v", path, want, got)
		}
	}
}

func comparePages(t *testing.T, label string, want, got SessionPage) {
	t.Helper()
	wantPaths := pathsOf(want.Items)
	gotPaths := pathsOf(got.Items)
	if !reflect.DeepEqual(wantPaths, gotPaths) {
		t.Errorf("%s: order differs\n catalog=%v\n mem    =%v", label, wantPaths, gotPaths)
		return
	}
	if (want.NextCursor == "") != (got.NextCursor == "") {
		t.Errorf("%s: cursor presence differs: catalog=%q mem=%q", label, want.NextCursor, got.NextCursor)
	}
	for i := range want.Items {
		w, g := want.Items[i], got.Items[i]
		if w.Turns != g.Turns || w.TopicTitle != g.TopicTitle || w.Preview != g.Preview ||
			w.LastActivityAt != g.LastActivityAt || w.Scope != g.Scope || w.Recovered != g.Recovered {
			t.Errorf("%s: item %d differs\n catalog=%+v\n mem    =%+v", label, i, w, g)
		}
	}
}

func compareWalk(t *testing.T, ctx context.Context, catalog *Catalog, mem *MemIndex, req SessionPageRequest) {
	t.Helper()
	var catalogAll, memAll []string
	cursor := ""
	for range 20 {
		page, err := catalog.ListSessions(ctx, SessionPageRequest{Scope: req.Scope, Limit: req.Limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("catalog walk: %v", err)
		}
		catalogAll = append(catalogAll, pathsOf(page.Items)...)
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	cursor = ""
	for range 20 {
		page, err := mem.ListSessions(SessionPageRequest{Scope: req.Scope, Limit: req.Limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("mem walk: %v", err)
		}
		memAll = append(memAll, pathsOf(page.Items)...)
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	if !reflect.DeepEqual(catalogAll, memAll) {
		t.Errorf("paged walk differs\n catalog=%v\n mem    =%v", catalogAll, memAll)
	}
	if len(catalogAll) == 0 {
		t.Error("paged walk returned nothing; the comparison proves nothing")
	}
}

func pathsOf(items []SessionRecord) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, filepath.Base(item.Path))
	}
	return out
}
