// costquote_wiring.go — where the frontend's sink acquires its cost quotes.
package boot

import (
	"strings"

	"reasonix/internal/billing"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/stats"
)

// quotedSink wraps the frontend sink so every host consumer — stats recorder,
// CLI metrics, ACP/eventwire bridges, Desktop — reads the same
// occurrence-time quote. Order from the agent:
//
//	Coalesce → GoalUsageTee → Sync → CostQuote → [Recorder] → frontend
func quotedSink(cfg *config.Config, opts Options) event.Sink {
	ctx := &event.QuoteContext{
		DisplayRequest: billing.DisplayRequest{
			Currency: cfg.ExplicitDisplayCurrency(),
			Source:   billing.DisplaySourceExplicit,
		},
		BillingModeForModel: func(modelRef string) string {
			entry, ok := cfg.ResolveModel(modelRef)
			if !ok {
				return ""
			}
			return entry.ProviderBillingMode()
		},
		// The vendor whose table applies is read off the endpoint's host, never
		// off what the entry is named — a relay serving the same model bills on
		// its own terms.
		CatalogProviderForModel: cfg.OfficialCatalogProvider,
	}
	// Innermost is the frontend sink; the recorder sits after quoting so history
	// JSONL stores the CostQuote.
	quoted := opts.Sink
	if source := strings.TrimSpace(opts.StatsSource); source != "" {
		quoted = stats.NewRecorder(quoted, config.StatsDir(), source)
	}
	return event.Sync(event.NewCostQuoteSink(quoted, ctx))
}
