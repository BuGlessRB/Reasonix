package eventwire

import (
	"encoding/json"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/model/billing"
)

// Ensures older clients still see cost/currency aliases while new clients get costQuote.
func TestToWireUsageDualWritesCostQuoteAndLegacyAliases(t *testing.T) {
	e := event.Event{
		Kind:     event.Usage,
		ModelRef: "deepseek-flash/deepseek-v4-flash",
		Usage:    &provider.Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000, TotalTokens: 2_000_000},
		Pricing:  &provider.Pricing{CacheHit: 0.02, Input: 1, Output: 4, Currency: "¥"},
		CostQuote: func() *billing.CostQuote {
			q := billing.BuildQuote(billing.QuoteInput{
				Usage:           billing.UsageTokens{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
				Rates:           billing.RateCard{CacheHit: 0.02, Input: 1, Output: 4, Currency: "CNY"},
				DisplayCurrency: "USD",
				ProviderKind:    "deepseek",
				ModelID:         "deepseek-flash",
			})
			return &q
		}(),
	}
	w := ToWire(e)
	if w.Usage == nil || w.Usage.CostQuote == nil {
		t.Fatal("missing costQuote")
	}
	if w.Usage.CostQuote.Valuations["USD"].Basis != billing.BasisOfficialTable {
		t.Fatalf("USD basis = %q, want official_table", w.Usage.CostQuote.Valuations["USD"].Basis)
	}
	if w.Usage.Cost <= 0 || w.Usage.CostUSD != w.Usage.Cost {
		t.Fatalf("legacy cost aliases = cost:%v costUsd:%v", w.Usage.Cost, w.Usage.CostUSD)
	}
	raw, err := json.Marshal(w.Usage)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("usage json invalid")
	}
	// Old clients ignore unknown fields; new field present.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["costQuote"]; !ok {
		t.Fatalf("costQuote missing from JSON: %s", raw)
	}
	if _, ok := m["cost"]; !ok {
		t.Fatalf("legacy cost missing: %s", raw)
	}
}

// The coverage identity is only worth having if it reaches the page that reads
// it, so this asserts the bytes rather than the struct the bytes came from.
func TestToWireUsageCarriesCoverageToTheJSON(t *testing.T) {
	usage := &provider.Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500}
	cases := []struct {
		name    string
		pricing *provider.Pricing
		want    string
	}{
		{"a priced round", &provider.Pricing{Input: 1.5, Output: 4.5, Currency: "¥"}, billing.CoverageComplete},
		{"a round with no price to quote", nil, billing.CoverageIncomplete},
	}
	for _, c := range cases {
		e := event.Event{Kind: event.Usage, ModelRef: "m", Usage: usage, Pricing: c.pricing}
		e.CostQuote = event.EnsureCostQuote(e, nil)
		raw, err := json.Marshal(ToWire(e).Usage)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			CostQuote struct {
				Coverage     string `json:"coverage"`
				CostComplete bool   `json:"costComplete"`
			} `json:"costQuote"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if m.CostQuote.Coverage != c.want {
			t.Errorf("%s: coverage on the wire = %q, want %q (%s)", c.name, m.CostQuote.Coverage, c.want, raw)
		}
		if m.CostQuote.CostComplete != (c.want == billing.CoverageComplete) {
			t.Errorf("%s: costComplete = %v against coverage %q", c.name, m.CostQuote.CostComplete, m.CostQuote.Coverage)
		}
	}
}
