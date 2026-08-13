package serve

import "testing"

// Three provider blocks on one endpoint (a two-model block plus a single-model
// block per model, each pinning its own price) offered the same two models
// four times.
func TestCollapseModelRoutesKeepsOneEntryPerEndpointModel(t *testing.T) {
	const ep = "https://api.deepseek.com"
	entries := []modelEntry{
		{Ref: "deepseek-flash/deepseek-v4-flash", Provider: "deepseek-flash", Model: "deepseek-v4-flash"},
		{Ref: "deepseek-pro/deepseek-v4-pro", Provider: "deepseek-pro", Model: "deepseek-v4-pro"},
		{Ref: "deepseek/deepseek-v4-flash", Provider: "deepseek", Model: "deepseek-v4-flash", Active: true},
		{Ref: "deepseek/deepseek-v4-pro", Provider: "deepseek", Model: "deepseek-v4-pro"},
	}
	routes := []modelRoute{
		{key: ep + "\x00deepseek-v4-flash", solo: true},
		{key: ep + "\x00deepseek-v4-pro", solo: true},
		{key: ep + "\x00deepseek-v4-flash"},
		{key: ep + "\x00deepseek-v4-pro"},
	}
	got := collapseModelRoutes(entries, routes)
	if len(got) != 2 {
		t.Fatalf("kept %d entries, want 2: %+v", len(got), got)
	}
	// Active wins so the current selection stays selectable; the other model
	// falls to the single-model block, whose price table is the exact one.
	if got[0].Ref != "deepseek-pro/deepseek-v4-pro" || got[1].Ref != "deepseek/deepseek-v4-flash" {
		t.Fatalf("kept %q and %q", got[0].Ref, got[1].Ref)
	}
}

// Same model name at two different vendors is two real choices.
func TestCollapseModelRoutesKeepsDistinctEndpoints(t *testing.T) {
	entries := []modelEntry{
		{Ref: "direct/gpt-x", Model: "gpt-x"},
		{Ref: "proxy/gpt-x", Model: "gpt-x"},
	}
	routes := []modelRoute{{key: "https://a\x00gpt-x"}, {key: "https://b\x00gpt-x"}}
	if got := collapseModelRoutes(entries, routes); len(got) != 2 {
		t.Fatalf("kept %d entries, want both vendors", len(got))
	}
}
