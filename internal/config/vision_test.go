package config

import "testing"

// A relay forwards someone else's models under its own name, so nothing here
// labels them. EffectiveVision answers no — the right default for deciding
// whether to send pixels — but the user has to be told which no it is, or the
// composer states a limitation of Claude that was never established.
func TestVisionDeclaredSeparatesNoFromNobodySaid(t *testing.T) {
	relay := &ProviderEntry{Name: "relay", Kind: "openai", BaseURL: "https://relay.example.com/v1", Model: "claude-sonnet-4-5"}
	if EffectiveVision(relay) {
		t.Fatal("an unlabelled relay model must not be sent images by default")
	}
	if VisionDeclared(relay) {
		t.Fatal("nobody labelled this model, so the panel must not report a settled answer")
	}

	ticked := &ProviderEntry{Name: "relay", Kind: "openai", BaseURL: "https://relay.example.com/v1",
		Model: "claude-sonnet-4-5", Models: []string{"claude-sonnet-4-5"}, VisionModels: []string{"claude-sonnet-4-5"}}
	if !EffectiveVision(ticked) || !VisionDeclared(ticked) {
		t.Fatal("ticking the model in the panel is what settles it, and it did not")
	}

	// An endpoint the kernel refuses images for has been answered, by the kernel.
	deepseek := &ProviderEntry{Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4"}
	if EffectiveVision(deepseek) || !VisionDeclared(deepseek) {
		t.Fatal("the kernel's own refusal is a settled answer, not a silence")
	}
}
