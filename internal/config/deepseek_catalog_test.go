package config

import (
	"slices"
	"testing"
)

// An installed user's model list is frozen in their config file, so a model the
// vendor added afterwards reaches them only if something puts it there. The
// bound on that is the same one every catalog migration takes: touch the entry
// that still carries what we shipped, and nothing else.
func TestDeepSeekVisionCatalogOnlyTouchesTheShippedList(t *testing.T) {
	cases := []struct {
		name   string
		entry  ProviderEntry
		want   bool
		models []string
		vision []string
	}{
		{
			name: "the list we shipped gains the model, ticked",
			entry: ProviderEntry{
				Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
				Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
			},
			want:   true,
			models: []string{"deepseek-v4-flash", "deepseek-v4-pro", DeepSeekVisionModel},
			vision: []string{DeepSeekVisionModel},
		},
		{
			name: "a curated list is the user's and stays put",
			entry: ProviderEntry{
				Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
				Models: []string{"deepseek-v4-pro"},
			},
			models: []string{"deepseek-v4-pro"},
		},
		{
			name: "a vision choice already made is not overwritten",
			entry: ProviderEntry{
				Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
				Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"}, VisionModels: []string{"deepseek-v4-pro"},
			},
			want:   true,
			models: []string{"deepseek-v4-flash", "deepseek-v4-pro", DeepSeekVisionModel},
			vision: []string{"deepseek-v4-pro"},
		},
		{
			name: "responses serves one model and not this one",
			entry: ProviderEntry{
				Name: "deepseek", Kind: "responses", BaseURL: "https://api.deepseek.com",
				Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
			},
			models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
		},
		{
			name: "a relay serving the same names is not this endpoint",
			entry: ProviderEntry{
				Name: "relay", Kind: "openai", BaseURL: "https://relay.example.com/v1",
				Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
			},
			models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := tc.entry
			if got := applyDeepSeekVisionCatalog(&entry); got != tc.want {
				t.Fatalf("changed = %v, want %v", got, tc.want)
			}
			if !slices.Equal(entry.ModelList(), tc.models) {
				t.Fatalf("models = %v, want %v", entry.ModelList(), tc.models)
			}
			if tc.vision != nil && !slices.Equal(entry.VisionModels, tc.vision) {
				t.Fatalf("vision_models = %v, want %v", entry.VisionModels, tc.vision)
			}
		})
	}
}

// Running twice must not append twice, because a plain load does not persist
// and the next one starts from the same file.
func TestDeepSeekVisionCatalogIsIdempotent(t *testing.T) {
	entry := ProviderEntry{
		Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
		Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
	}
	applyDeepSeekVisionCatalog(&entry)
	if applyDeepSeekVisionCatalog(&entry) {
		t.Fatal("a second pass reported a change; the list would grow every load")
	}
	if got := len(entry.ModelList()); got != 3 {
		t.Fatalf("models = %v, want three", entry.ModelList())
	}
}

// The protocol upgrade allow-lists the models it shipped. Backfilling a third
// one must not read as curation, or the migration would strand exactly the
// users it just reached.
func TestVisionBackfillKeepsTheProtocolUpgradeAvailable(t *testing.T) {
	entry := ProviderEntry{
		Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
		APIKeyEnv: "DEEPSEEK_API_KEY", Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
	}
	if !CanUpgradeDeepSeekProviderProtocol(&entry) {
		t.Fatal("fixture does not start upgradable")
	}
	applyDeepSeekVisionCatalog(&entry)
	if !CanUpgradeDeepSeekProviderProtocol(&entry) {
		t.Fatal("the backfill froze this entry out of the protocol upgrade")
	}
}

// The notice a user gets when an attachment cannot be read has to know whether
// anything could read it; saying "handed to a delegate" with nothing configured
// is how a dropped picture reads as a delivered one.
func TestFirstVisionModelRefFindsAReaderOrSaysNone(t *testing.T) {
	none := &Config{Providers: []ProviderEntry{{
		Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
		Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
	}}}
	if got := none.FirstVisionModelRef(); got != "" {
		t.Fatalf("text-only config offered %q", got)
	}

	withVision := &Config{Providers: []ProviderEntry{{
		Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
		Models:       []string{"deepseek-v4-flash", "deepseek-v4-pro", DeepSeekVisionModel},
		VisionModels: []string{DeepSeekVisionModel},
	}}}
	if got := withVision.FirstVisionModelRef(); got != "deepseek/"+DeepSeekVisionModel {
		t.Fatalf("offered %q, want the image-taking model", got)
	}
}
