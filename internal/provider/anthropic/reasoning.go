// Reasoning contract resolution: which thinking fields this endpoint accepts.
package anthropic

import (
	"strings"

	"reasonix/internal/provider"
)

// reasoningProtocolAnthropic is the config-declared name for Anthropic's own
// extended-thinking contract; see config.ReasoningProtocolAnthropic.
const reasoningProtocolAnthropic = "anthropic"

// resolveReasoning turns the configured knobs into the thinking mode and effort
// this endpoint's contract accepts. Anthropic's own fields — adaptive thinking,
// display, output_config.effort — need a first-party endpoint or a declared
// protocol; every other gateway answers the plain enabled|disabled toggle.
func resolveReasoning(root string, extra map[string]any) (thinking, effort string) {
	thinking = lowerExtra(extra, "thinking")
	effort = lowerExtra(extra, "effort")
	depth := provider.IsNativeAnthropic(root) ||
		lowerExtra(extra, "reasoning_protocol") == reasoningProtocolAnthropic
	switch {
	case !depth && thinking == "adaptive":
		// A value only Anthropic's contract defines, configured before this
		// endpoint's was known. Fall back to the provider default instead of
		// guessing a toggle spelling the relay may reject just as hard.
		thinking = ""
	case depth && thinking == "" && depthEffort(effort):
		// Depth means nothing without extended thinking, so the level engages
		// it — nothing outside this package writes a mode into the user's
		// config to make /effort work.
		thinking = "adaptive"
	}
	return thinking, effort
}

// depthEffort reports whether a level names a reasoning depth rather than the
// binary thinking toggle a compatible gateway takes.
func depthEffort(effort string) bool {
	switch effort {
	case "", "enabled", "disabled", "adaptive", "none", "off":
		return false
	default:
		return true
	}
}

func lowerExtra(extra map[string]any, key string) string {
	value, _ := extra[key].(string)
	return strings.ToLower(strings.TrimSpace(value))
}
