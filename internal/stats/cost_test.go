package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/testenv"
)

// writeDay drops raw JSONL lines into a day file the way the recorder would.
func writeDay(t *testing.T, dir, day string, lines ...map[string]any) {
	t.Helper()
	var buf []byte
	for _, line := range lines {
		encoded, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(append(buf, encoded...), '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, day+".jsonl"), buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func queryAll(t *testing.T, dir string) RangeStats {
	t.Helper()
	from, _ := time.Parse(dayLayout, "2026-08-01")
	to, _ := time.Parse(dayLayout, "2026-08-31")
	got, err := NewWriter(dir).Query(SourceFilter{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Two billing currencies never add up: one number would have to invent an
// exchange rate, and the rate a turn was billed at is not the rate today.
func TestCostStaysSeparatePerCurrency(t *testing.T) {
	dir := testenv.TempDir(t)
	writeDay(t, dir, "2026-08-10",
		map[string]any{"ts": "2026-08-10T01:00:00Z", "model": "deepseek/x", "source": "cli", "total": 100, "cost_amount": "1.50", "cost_currency": "CNY"},
		map[string]any{"ts": "2026-08-10T02:00:00Z", "model": "other/y", "source": "cli", "total": 200, "cost_amount": "0.25", "cost_currency": "USD"},
		map[string]any{"ts": "2026-08-10T03:00:00Z", "model": "deepseek/x", "source": "cli", "total": 100, "cost_amount": "2.50", "cost_currency": "CNY"},
	)
	got := queryAll(t, dir)
	if len(got.Cost) != 2 {
		t.Fatalf("cost = %+v, want one entry per currency", got.Cost)
	}
	if got.Cost[0].Currency != "CNY" || got.Cost[0].Amount != "4" {
		t.Errorf("CNY total = %+v, want 4", got.Cost[0])
	}
	if got.Cost[1].Currency != "USD" || got.Cost[1].Amount != "0.25" {
		t.Errorf("USD total = %+v, want 0.25", got.Cost[1])
	}
}

// A day with tokens but no cost field is not a day that cost nothing — the
// field was added later, and rendering it as zero would understate the bill.
func TestDaysWithoutCostReportNoneRatherThanZero(t *testing.T) {
	dir := testenv.TempDir(t)
	writeDay(t, dir, "2026-08-10", map[string]any{"ts": "2026-08-10T01:00:00Z", "model": "deepseek/x", "source": "cli", "total": 5000})
	writeDay(t, dir, "2026-08-11", map[string]any{"ts": "2026-08-11T01:00:00Z", "model": "deepseek/x", "source": "cli", "total": 10, "cost_amount": "0.75", "cost_currency": "CNY"})
	got := queryAll(t, dir)
	var priced, unpriced int
	for _, day := range got.Daily {
		switch day.Day {
		case "2026-08-10":
			if len(day.Cost) != 0 {
				t.Errorf("a day with no cost field reported %+v", day.Cost)
			}
			unpriced++
		case "2026-08-11":
			if len(day.Cost) != 1 || day.Cost[0].Amount != "0.75" {
				t.Errorf("priced day = %+v, want 0.75 CNY", day.Cost)
			}
			priced++
		}
	}
	if priced != 1 || unpriced != 1 {
		t.Fatalf("expected both days in the range, saw priced=%d unpriced=%d", priced, unpriced)
	}
	if len(got.Cost) != 1 || got.Cost[0].Amount != "0.75" {
		t.Errorf("range total = %+v, want only the priced day", got.Cost)
	}
}

// An amount that does not parse is dropped, not guessed at: a wrong total is
// worse than a missing one.
func TestUnparseableAmountIsSkipped(t *testing.T) {
	dir := testenv.TempDir(t)
	writeDay(t, dir, "2026-08-10",
		map[string]any{"ts": "2026-08-10T01:00:00Z", "model": "deepseek/x", "source": "cli", "total": 10, "cost_amount": "not-a-number", "cost_currency": "CNY"},
		map[string]any{"ts": "2026-08-10T02:00:00Z", "model": "deepseek/x", "source": "cli", "total": 10, "cost_amount": "1.25", "cost_currency": "CNY"},
	)
	got := queryAll(t, dir)
	if len(got.Cost) != 1 || got.Cost[0].Amount != "1.25" {
		t.Fatalf("cost = %+v, want only the parseable row", got.Cost)
	}
}
