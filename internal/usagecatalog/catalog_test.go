package usagecatalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReconcileFileAndDuplicateReceiptAreIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-08-10.jsonl")
	line := []byte("{\"ts\":\"2026-08-10T10:00:00+08:00\",\"model\":\"deepseek/model\",\"source\":\"desktop\",\"total\":42}\n")
	if err := os.WriteFile(path, line, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileFile(ctx, path, "2026-08-10"); err != nil {
		t.Fatal(err)
	}
	rows, err := catalog.Query(ctx, "2026-08-10", "2026-08-10", "desktop")
	if err != nil || len(rows) != 1 || rows[0].Total != 42 || rows[0].Requests != 1 {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	var receipt AppendReceipt
	if err := catalog.db.QueryRow(`SELECT file_path,day,byte_offset,byte_length,line_hash FROM usage_records WHERE file_path=?`, path).Scan(
		&receipt.Path, &receipt.Day, &receipt.Offset, &receipt.Length, &receipt.LineHash); err != nil {
		t.Fatal(err)
	}
	entry := Entry{Day: receipt.Day, Source: "desktop", ModelRef: "deepseek/model", Provider: "deepseek", Total: 42, Requests: 1}
	if err := catalog.applyReceipt(ctx, receipt, entry); err != nil {
		t.Fatal(err)
	}
	rows, err = catalog.Query(ctx, "2026-08-10", "2026-08-10", "desktop")
	if err != nil || len(rows) != 1 || rows[0].Total != 42 {
		t.Fatalf("duplicate changed aggregate: rows=%#v err=%v", rows, err)
	}
}

func TestReadyRejectsExternalAppendUntilReconciled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-08-10.jsonl")
	if err := os.WriteFile(path, []byte("{\"ts\":\"2026-08-10T10:00:00Z\",\"total\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileFile(ctx, path, "2026-08-10"); err != nil {
		t.Fatal(err)
	}
	if !catalog.Ready(ctx, dir, []string{"2026-08-10"}) {
		t.Fatal("catalog should be ready")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{\"ts\":\"2026-08-10T11:00:00Z\",\"total\":2}\n")
	_ = f.Close()
	if catalog.Ready(ctx, dir, []string{"2026-08-10"}) {
		t.Fatal("external append was treated as indexed")
	}
}

func TestReadyRejectsSameSizeRewriteWithDifferentMtime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-08-10.jsonl")
	original := []byte("{\"ts\":\"2026-08-10T10:00:00Z\",\"total\":1,\"source\":\"desktop\"}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileFile(ctx, path, "2026-08-10"); err != nil {
		t.Fatal(err)
	}
	if !catalog.Ready(ctx, dir, []string{"2026-08-10"}) {
		t.Fatal("catalog should be ready after reconcile")
	}
	// Same byte length, different content and mtime — Ready must fail so JSONL
	// remains the authority until the projection is rescanned.
	replacement := []byte("{\"ts\":\"2026-08-10T12:00:00Z\",\"total\":9,\"source\":\"desktop\"}\n")
	if len(replacement) != len(original) {
		t.Fatalf("test fixture length mismatch: %d vs %d", len(replacement), len(original))
	}
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	// Force a distinct mtime on filesystems with coarse timestamps.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if catalog.Ready(ctx, dir, []string{"2026-08-10"}) {
		t.Fatal("same-size rewrite was treated as still ready")
	}
}

// Two currencies on one key must not become one number. The JSONL path in
// internal/stats enforces this and is covered there; the catalog is what
// answers once it is ready, so the same invariant needs proving here.
func TestCostsStaySeparatePerCurrencyInTheProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-08-10.jsonl")
	lines := "" +
		`{"ts":"2026-08-10T10:00:00+08:00","model":"deepseek/model","source":"cli","total":10,"cost_amount":"1.50","cost_currency":"CNY"}` + "\n" +
		`{"ts":"2026-08-10T11:00:00+08:00","model":"deepseek/model","source":"cli","total":10,"cost_amount":"0.25","cost_currency":"USD"}` + "\n" +
		`{"ts":"2026-08-10T12:00:00+08:00","model":"deepseek/model","source":"cli","total":10,"cost_amount":"2.50","cost_currency":"CNY"}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileFile(ctx, path, "2026-08-10"); err != nil {
		t.Fatal(err)
	}
	rows, err := catalog.Query(ctx, "2026-08-10", "2026-08-10", "cli")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want the three records folded onto one key", len(rows))
	}
	if rows[0].Total != 30 {
		t.Errorf("tokens = %d, want 30 — those do add up", rows[0].Total)
	}
	got := map[string]int64{}
	for _, entry := range rows[0].Costs {
		got[entry.Currency] = entry.Amount
	}
	if len(got) != 2 {
		t.Fatalf("costs = %+v, want one entry per currency", rows[0].Costs)
	}
	// billing.Amount is fixed-point at 1e9.
	if got["CNY"] != 4_000_000_000 {
		t.Errorf("CNY = %d, want 4.00", got["CNY"])
	}
	if got["USD"] != 250_000_000 {
		t.Errorf("USD = %d, want 0.25", got["USD"])
	}
}

// A currency spelled differently in two rows is one currency. The two read
// paths normalize at different moments, and a panel showing "¥1.50 · ¥2.50"
// for one currency is what that costs.
func TestCurrencySpellingIsNormalizedOnIngest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-08-11.jsonl")
	lines := "" +
		`{"ts":"2026-08-11T10:00:00+08:00","model":"deepseek/model","source":"cli","total":5,"cost_amount":"1.00","cost_currency":"cny"}` + "\n" +
		`{"ts":"2026-08-11T11:00:00+08:00","model":"deepseek/model","source":"cli","total":5,"cost_amount":"1.00","cost_currency":"CNY"}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileFile(ctx, path, "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	rows, err := catalog.Query(ctx, "2026-08-11", "2026-08-11", "cli")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	if len(rows[0].Costs) != 1 || rows[0].Costs[0].Amount != 2_000_000_000 {
		t.Fatalf("costs = %+v, want a single CNY entry of 2.00", rows[0].Costs)
	}
}
