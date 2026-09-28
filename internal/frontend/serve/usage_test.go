package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/state/stats"
)

// The usage panel reads money in the currency the session's own costs read in,
// so a CNY wallet sees a CNY total wherever the vendor published a CNY price.
func TestUsageReadsInTheSessionDisplayCurrency(t *testing.T) {
	s := newListenerTestServer(t)
	now := time.Now()
	line := map[string]any{"ts": now.Format(time.RFC3339), "model": "deepseek-flash/deepseek-flash", "source": "desktop",
		"total": 100, "cost_amount": "0.5", "cost_currency": "USD", "valuation_usd": "0.5", "valuation_cny": "3.5"}
	encoded, _ := json.Marshal(line)
	if err := os.MkdirAll(config.StatsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.StatsDir(), now.Format("2006-01-02")+".jsonl"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	read := func() stats.RangeStats {
		rec := httptest.NewRecorder()
		s.usage(rec, httptest.NewRequest(http.MethodGet, "/usage?days=1", nil))
		var got stats.RangeStats
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("status %d: %v", rec.Code, err)
		}
		return got
	}
	if got := read().Cost; len(got) != 1 || got[0].Currency != "USD" || got[0].Amount != "0.5" {
		t.Fatalf("no display currency = %+v, want the billed USD", got)
	}
	s.bc.SetDisplayCurrency("CNY")
	if got := read().Cost; len(got) != 1 || got[0].Currency != "CNY" || got[0].Amount != "3.5" {
		t.Fatalf("CNY display = %+v, want the vendor's CNY price", got)
	}
}
