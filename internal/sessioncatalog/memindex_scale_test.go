package sessioncatalog

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func realTargets(t *testing.T) []DirectoryTarget {
	t.Helper()
	home := os.Getenv("HOME")
	var targets []DirectoryTarget
	global := filepath.Join(home, ".reasonix", "sessions")
	if _, err := os.Stat(global); err == nil {
		targets = append(targets, DirectoryTarget{Path: global, Scope: "global"})
	}
	projects := filepath.Join(home, ".reasonix", "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return targets
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(projects, e.Name(), "sessions")
		matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if len(matches) == 0 {
			continue
		}
		targets = append(targets, DirectoryTarget{Path: dir, Scope: "project", WorkspaceRoot: "/root/" + e.Name()})
	}
	return targets
}

// TestScaleRealWorkspaces measures both projections against whatever session
// tree this machine actually has, because the shape that matters is not one big
// directory but many small ones: 461 workspaces averaging a couple of sessions
// each is what a working install looks like. Skipped where there is no tree.
func TestScaleRealWorkspaces(t *testing.T) {
	targets := realTargets(t)
	if len(targets) < 2 {
		t.Skip("no real multi-workspace session tree")
	}
	ctx := context.Background()
	now := time.Now

	catalog, err := Open(ctx, Options{Path: filepath.Join(t.TempDir(), "scale.sqlite"), DisableRepair: true})
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close(context.Background())

	start := time.Now()
	for _, target := range targets {
		if err := catalog.ReconcileDirectory(ctx, target); err != nil {
			t.Fatalf("reconcile %s: %v", target.Path, err)
		}
	}
	catalogBuild := time.Since(start)

	mem := NewMemIndex(now)
	start = time.Now()
	for _, target := range targets {
		if err := mem.ScanDirectory(target); err != nil {
			t.Fatalf("scan %s: %v", target.Path, err)
		}
	}
	memBuild := time.Since(start)

	// Warm rebuild: the catalog skips directories whose signature is unchanged;
	// the memory index has no such shortcut and rescans everything.
	start = time.Now()
	for _, target := range targets {
		_ = catalog.ReconcileDirectory(ctx, target)
	}
	catalogWarm := time.Since(start)
	start = time.Now()
	for _, target := range targets {
		_ = mem.ScanDirectory(target)
	}
	memRescan := time.Since(start)

	req := SessionPageRequest{Scope: "all", Limit: 50}
	var catalogQuery, memQuery time.Duration
	var catalogItems, memItems int
	for range 10 {
		start = time.Now()
		page, err := catalog.ListSessions(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		catalogQuery += time.Since(start)
		catalogItems = len(page.Items)

		start = time.Now()
		mpage, err := mem.ListSessions(req)
		if err != nil {
			t.Fatal(err)
		}
		memQuery += time.Since(start)
		memItems = len(mpage.Items)
	}

	// One workspace at a time is the common sidebar read.
	single := SessionPageRequest{Scope: "all", Directory: targets[0].Path, Limit: 50}
	var catalogOne, memOne time.Duration
	for range 10 {
		start = time.Now()
		_, _ = catalog.ListSessions(ctx, single)
		catalogOne += time.Since(start)
		start = time.Now()
		_, _ = mem.ListSessions(single)
		memOne += time.Since(start)
	}

	t.Logf("directories=%d", len(targets))
	t.Logf("build      catalog=%v      mem=%v", catalogBuild.Round(time.Millisecond), memBuild.Round(time.Millisecond))
	t.Logf("rebuild    catalog=%v(skip) mem=%v(full rescan)", catalogWarm.Round(time.Millisecond), memRescan.Round(time.Millisecond))
	t.Logf("query all  catalog=%v      mem=%v   (items %d/%d)", (catalogQuery / 10).Round(time.Microsecond), (memQuery / 10).Round(time.Microsecond), catalogItems, memItems)
	t.Logf("query one  catalog=%v      mem=%v", (catalogOne / 10).Round(time.Microsecond), (memOne / 10).Round(time.Microsecond))

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	held := 0
	for _, records := range mem.byDir {
		held += len(records)
	}
	t.Logf("mem index holds %d records, heap in use %.1f MB", held, float64(after.HeapInuse)/(1<<20))
}
