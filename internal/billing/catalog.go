package billing

import (
	"net/url"
	"strings"
	"time"
)

// Official catalog metadata: currency, model, effective dates, documentation
// sources, and price fingerprints. User-custom prices always win over catalog.

// CatalogEntry is one official list price for a model in a billing currency.
type CatalogEntry struct {
	Provider      string // deepseek | longcat | mimo
	Model         string
	Currency      string // ISO billing currency for this row
	CacheHit      float64
	Input         float64
	Output        float64
	EffectiveFrom string // YYYY-MM-DD inclusive, empty = unbounded
	EffectiveTo   string // YYYY-MM-DD exclusive, empty = unbounded
	DocURL        string
	BillingMode   string // payg | subscription_equivalent
	Notes         string
	Fingerprint   string // filled by Register
	// Peak is what the same tokens cost inside Window. Nil when the vendor
	// bills one rate around the clock, and then the fields above are the rate.
	Peak   *RateCard
	Window *PeakWindow
	// CheckedOn is when a person last read DocURL and confirmed these numbers.
	// No vendor here serves prices over an API, so this date is all that stands
	// between a price change and a readout that is quietly wrong.
	CheckedOn string
}

// deepseekPeak is the schedule DeepSeek publishes: peak on weekday mornings and
// afternoons, and — since 2026-08-23 — never at a weekend.
var deepseekPeak = &PeakWindow{
	OffsetSeconds:  beijing,
	Hours:          [][2]int{{9, 12}, {14, 18}},
	WeekendOffPeak: "2026-08-23",
}

// checkedOn is when the tables below were last read off the vendors' pages.
const checkedOn = "2026-08-22"

// CatalogSourceURLs document public pricing pages.
const (
	DocDeepSeekPricing   = "https://api-docs.deepseek.com/quick_start/pricing"
	DocLongCatPricingUSD = "https://longcat.chat/platform/docs/Pricing/LongCat-2.0.html"
	DocLongCatPricingCNY = "https://longcat.chat/platform/docs/zh/pricing/long-cat-2.0"
	DocMiMoPAYG          = "https://mimo.mi.com/docs/price/pay-as-you-go"
	DocMiMoTokenPlan     = "https://platform.xiaomimimo.com/token-plan"
)

// officialEndpointHosts maps a vendor's own API hostnames to the catalog
// provider whose list prices apply there. Host equality is the test: a reseller
// or proxy bills on its own terms even when it serves the same model, and an
// entry merely named after a vendor is not evidence of the vendor's endpoint.
var officialEndpointHosts = map[string]string{
	"api.deepseek.com":              "deepseek",
	"api.longcat.chat":              "longcat",
	"api.xiaomimimo.com":            "mimo",
	"token-plan-cn.xiaomimimo.com":  "mimo",
	"token-plan-sgp.xiaomimimo.com": "mimo",
	"token-plan-ams.xiaomimimo.com": "mimo",
}

// OfficialProviderForEndpoint returns the catalog provider whose official
// prices apply to baseURL, or "" when the address is not a vendor's own API.
func OfficialProviderForEndpoint(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return officialEndpointHosts[strings.ToLower(u.Hostname())]
}

// OfficialCatalog is the built-in price book. Rates match config defaults.
func OfficialCatalog() []CatalogEntry {
	entries := []CatalogEntry{
		// DeepSeek regional tables (official pricing doc). The base rate is the
		// off-peak one, which is what a caller charging a single rate should
		// quote; Peak is the same tokens inside deepseekPeak's hours.
		{Provider: "deepseek", Model: "deepseek-v4-flash", Currency: "CNY", CacheHit: 0.05, Input: 1.5, Output: 4.5, Peak: &RateCard{CacheHit: 0.10, Input: 3.0, Output: 9.0}, Window: deepseekPeak, DocURL: DocDeepSeekPricing, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		{Provider: "deepseek", Model: "deepseek-v4-pro", Currency: "CNY", CacheHit: 0.15, Input: 4.5, Output: 13.5, Peak: &RateCard{CacheHit: 0.30, Input: 9.0, Output: 27.0}, Window: deepseekPeak, DocURL: DocDeepSeekPricing, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		{Provider: "deepseek", Model: "deepseek-v4-flash", Currency: "USD", CacheHit: 0.007, Input: 0.22, Output: 0.66, Peak: &RateCard{CacheHit: 0.014, Input: 0.44, Output: 1.32}, Window: deepseekPeak, DocURL: DocDeepSeekPricing, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		{Provider: "deepseek", Model: "deepseek-v4-pro", Currency: "USD", CacheHit: 0.022, Input: 0.66, Output: 1.98, Peak: &RateCard{CacheHit: 0.044, Input: 1.32, Output: 3.96}, Window: deepseekPeak, DocURL: DocDeepSeekPricing, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		// Vision bills at the flash rates; an image is charged as the tokens it
		// scales to, so it arrives already counted in the prompt total and needs
		// no per-image rate of its own.
		{Provider: "deepseek", Model: "deepseek-v4-flash-vision-exp", Currency: "CNY", CacheHit: 0.05, Input: 1.5, Output: 4.5, Peak: &RateCard{CacheHit: 0.10, Input: 3.0, Output: 9.0}, Window: deepseekPeak, DocURL: DocDeepSeekPricing, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		{Provider: "deepseek", Model: "deepseek-v4-flash-vision-exp", Currency: "USD", CacheHit: 0.007, Input: 0.22, Output: 0.66, Peak: &RateCard{CacheHit: 0.014, Input: 0.44, Output: 1.32}, Window: deepseekPeak, DocURL: DocDeepSeekPricing, BillingMode: BillingModePAYG, CheckedOn: checkedOn},

		// LongCat dual currency tables: the launch discount, for which the vendor
		// publishes no end date. List price is CNY 0.10/5/20, USD 0.015/0.75/2.95
		// — what these revert to, and why CheckedOn earns its place here.
		{Provider: "longcat", Model: "LongCat-2.0", Currency: "CNY", CacheHit: 0.04, Input: 2, Output: 8, DocURL: DocLongCatPricingCNY, BillingMode: BillingModePAYG, CheckedOn: checkedOn, Notes: "launch_discount_no_end_date"},
		{Provider: "longcat", Model: "LongCat-2.0", Currency: "USD", CacheHit: 0.006, Input: 0.30, Output: 1.20, DocURL: DocLongCatPricingUSD, BillingMode: BillingModePAYG, CheckedOn: checkedOn, Notes: "launch_discount_no_end_date"},

		// MiMo domestic PAYG; Token Plan uses the same rates as subscription_equivalent.
		{Provider: "mimo", Model: "mimo-v2.5-pro", Currency: "CNY", CacheHit: 0.025, Input: 3, Output: 6, DocURL: DocMiMoPAYG, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		{Provider: "mimo", Model: "mimo-v2.5", Currency: "CNY", CacheHit: 0.02, Input: 1, Output: 2, DocURL: DocMiMoPAYG, BillingMode: BillingModePAYG, CheckedOn: checkedOn},
		// Not on the vendor's price page as of CheckedOn. Kept because removing a
		// rate is what turns a still-configured model's cost silently into zero;
		// a stale rate at least reads as a number somebody can question.
		{Provider: "mimo", Model: "mimo-v2-flash", Currency: "CNY", CacheHit: 0.07, Input: 0.70, Output: 2.10, DocURL: DocMiMoPAYG, BillingMode: BillingModePAYG, CheckedOn: checkedOn, Notes: "not_on_vendor_page"},
		{Provider: "mimo", Model: "mimo-v2.5-pro", Currency: "CNY", CacheHit: 0.025, Input: 3, Output: 6, DocURL: DocMiMoTokenPlan, BillingMode: BillingModeSubscriptionEquivalent, Notes: "payg_equivalent_not_plan_bill", CheckedOn: checkedOn},
		{Provider: "mimo", Model: "mimo-v2.5", Currency: "CNY", CacheHit: 0.02, Input: 1, Output: 2, DocURL: DocMiMoTokenPlan, BillingMode: BillingModeSubscriptionEquivalent, Notes: "payg_equivalent_not_plan_bill", CheckedOn: checkedOn},
	}
	for i := range entries {
		entries[i].Fingerprint = PricingFingerprint(RateCard{
			CacheHit: entries[i].CacheHit,
			Input:    entries[i].Input,
			Output:   entries[i].Output,
			Currency: entries[i].Currency,
		})
	}
	return entries
}

// LookupCatalog finds an official entry. billingMode empty matches payg first.
func LookupCatalog(provider, model, currency, billingMode string) (CatalogEntry, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	currency = NormalizeCurrency(currency)
	billingMode = strings.TrimSpace(billingMode)
	if billingMode == "" {
		billingMode = BillingModePAYG
	}
	var fallback CatalogEntry
	foundFallback := false
	for _, e := range OfficialCatalog() {
		if e.Provider != provider || e.Model != model {
			continue
		}
		if NormalizeCurrency(e.Currency) != currency {
			continue
		}
		if e.BillingMode == billingMode {
			return e, true
		}
		if e.BillingMode == BillingModePAYG {
			fallback = e
			foundFallback = true
		}
	}
	if foundFallback {
		return fallback, true
	}
	return CatalogEntry{}, false
}

// RateCardFromCatalog builds a RateCard from a catalog entry's base rate.
func RateCardFromCatalog(e CatalogEntry) RateCard {
	return RateCard{
		CacheHit: e.CacheHit,
		Input:    e.Input,
		Output:   e.Output,
		Currency: e.Currency,
	}
}

// RatesAt is what this entry charges at t: the peak card inside its window, the
// base card everywhere else. An entry without a window charges one rate, and
// then t does not matter.
func (e CatalogEntry) RatesAt(t time.Time) RateCard {
	if e.Peak == nil || !e.Window.IsPeak(t) {
		return RateCardFromCatalog(e)
	}
	peak := *e.Peak
	peak.Currency = e.Currency
	return peak
}

// InEffect reports whether this entry's dates cover the day t falls on.
func (e CatalogEntry) InEffect(t time.Time) bool {
	day := t.UTC().Format("2006-01-02")
	if e.EffectiveFrom != "" && day < e.EffectiveFrom {
		return false
	}
	return e.EffectiveTo == "" || day < e.EffectiveTo
}

// MatchesCatalog reports whether rates equal a known official entry.
func MatchesCatalog(provider, model string, rates RateCard) (CatalogEntry, bool) {
	cur := NormalizeCurrency(rates.Currency)
	for _, e := range OfficialCatalog() {
		if e.Provider != strings.ToLower(strings.TrimSpace(provider)) {
			continue
		}
		if e.Model != strings.TrimSpace(model) {
			continue
		}
		if NormalizeCurrency(e.Currency) != cur {
			continue
		}
		// Either card identifies the vendor's own price. Which one a user
		// happens to have configured says nothing about when they are billing:
		// the hour decides that, and RatesAt is what reads it.
		if e.CacheHit == rates.CacheHit && e.Input == rates.Input && e.Output == rates.Output {
			return e, true
		}
		if p := e.Peak; p != nil && p.CacheHit == rates.CacheHit && p.Input == rates.Input && p.Output == rates.Output {
			return e, true
		}
	}
	return CatalogEntry{}, false
}
