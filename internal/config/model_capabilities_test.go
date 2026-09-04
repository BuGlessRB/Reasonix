package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/provider"
)

func TestModelCapabilityResolverHonorsExplicitConfigBeforeCache(t *testing.T) {
	r := &ModelCapabilityResolver{entries: map[string]ModelCapabilityCacheEntry{}}
	entry := ProviderEntry{Name: "custom", Kind: "openai", BaseURL: "https://example.test", Model: "mixed"}
	r.PutCatalog(entry, []provider.ModelInfo{{ID: "mixed", InputModalities: []provider.ModelModality{provider.ModalityText, provider.ModalityImage}}})
	if got := r.Resolve(&entry); got.State != CapabilitySupported || got.Source != CapabilitySourceAdapter {
		t.Fatalf("adapter capability = %+v", got)
	}
	entry.VisionModels = []string{"mixed"}
	if got := r.Resolve(&entry); got.State != CapabilitySupported || got.Source != CapabilitySourceLegacy {
		t.Fatalf("legacy capability = %+v", got)
	}
	entry.visionOverride = capabilityBoolPtr(false)
	if got := r.Resolve(&entry); got.State != CapabilityUnsupported || got.Source != CapabilitySourceOverride {
		t.Fatalf("override capability = %+v", got)
	}
}

func TestModelCapabilityResolverDefaultsExactModelsToText(t *testing.T) {
	r := &ModelCapabilityResolver{entries: map[string]ModelCapabilityCacheEntry{}}
	entry := ProviderEntry{Name: "custom", Kind: "openai", BaseURL: "https://example.test", Model: "new-model"}
	got := r.Resolve(&entry)
	if got.State != CapabilityUnsupported || got.Source != CapabilitySourceDefault || len(got.InputModalities) != 1 || got.InputModalities[0] != provider.ModalityText {
		t.Fatalf("default capability = %+v", got)
	}
}

func TestModelCapabilityResolverLoadsIndependentCache(t *testing.T) {
	dir := t.TempDir()
	oldCache := os.Getenv("REASONIX_CACHE_HOME")
	if err := os.Setenv("REASONIX_CACHE_HOME", dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("REASONIX_CACHE_HOME", oldCache) })

	entry := ProviderEntry{Name: "custom", Kind: "openai", BaseURL: "https://example.test", Model: "vision"}
	writer := NewModelCapabilityResolver()
	writer.PutCatalog(entry, []provider.ModelInfo{{ID: "vision", InputModalities: []provider.ModelModality{provider.ModalityText, provider.ModalityImage}}})
	reader := NewModelCapabilityResolver()
	got := reader.Resolve(&entry)
	if got.State != CapabilitySupported || got.Source != CapabilitySourceAdapter {
		t.Fatalf("reloaded capability = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "model-capabilities-v1.json")); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
}

func TestModelCapabilityResolverIgnoresExpiredCache(t *testing.T) {
	r := &ModelCapabilityResolver{entries: map[string]ModelCapabilityCacheEntry{}}
	entry := ProviderEntry{Name: "custom", Kind: "openai", BaseURL: "https://example.test", Model: "vision"}
	key := r.entryKey(&entry, entry.Model)
	r.entries[key] = ModelCapabilityCacheEntry{
		ProviderFingerprint: r.providerFingerprint(entry), ModelID: entry.Model,
		InputModalities: []provider.ModelModality{provider.ModalityText, provider.ModalityImage},
		ExpiresAt:       time.Now().Add(-time.Minute), Source: CapabilitySourceCache,
	}
	got := r.Resolve(&entry)
	if got.Source != CapabilitySourceDefault || got.State != CapabilityUnsupported {
		t.Fatalf("expired cache capability = %+v", got)
	}
}

func capabilityBoolPtr(value bool) *bool { return &value }
