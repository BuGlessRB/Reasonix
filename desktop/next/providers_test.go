package main

import (
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/provider"
)

// Provider kinds register themselves from init, so a binary only has the ones it
// links. The shell shipped without these imports and every Anthropic model in
// the picker failed at switch time with "unknown kind" — the settings pane could
// offer it because the catalog reads config, which does not need the factory.
func TestEveryProviderKindTheConfigAcceptsIsLinked(t *testing.T) {
	registered := map[string]bool{}
	for _, kind := range provider.Kinds() {
		registered[kind] = true
	}
	// The kinds providerEntryFrom lets a user save, plus the ones the curated
	// presets ship with.
	for _, kind := range []string{"openai", "anthropic", "responses"} {
		if !registered[kind] {
			t.Errorf("provider kind %q is not linked into the shell, so switching to one fails at runtime", kind)
		}
	}
}

// Whatever the presets can write into a config, this binary has to be able to
// build. A preset naming an unlinked kind is a source the user can add and then
// never select.
func TestEveryPresetKindIsLinked(t *testing.T) {
	registered := map[string]bool{}
	for _, kind := range provider.Kinds() {
		registered[kind] = true
	}
	for _, preset := range config.CuratedProviderPresets() {
		for _, entry := range preset.Entries {
			if entry.Kind != "" && !registered[entry.Kind] {
				t.Errorf("preset %q wants kind %q, which is not linked into the shell", preset.ID, entry.Kind)
			}
		}
	}
}
