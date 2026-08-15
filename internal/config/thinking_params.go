package config

import "strings"

// reasoningProtocolNone is the request shape that carries no thinking or
// reasoning control fields at all.
const reasoningProtocolNone = "none"

// CanConfigureThinkingParams reports whether reasoning_protocol governs this
// entry's request shape. Only the OpenAI-compatible wire carries the
// thinking/reasoning_effort fields a relay can reject outright.
func CanConfigureThinkingParams(e *ProviderEntry) bool {
	return e != nil && strings.EqualFold(strings.TrimSpace(e.Kind), "openai")
}

// SendsThinkingParams reports whether this endpoint may receive thinking
// controls. False means the user pinned the plain-chat shape because the
// gateway rejects those fields.
func SendsThinkingParams(e *ProviderEntry) bool {
	if !CanConfigureThinkingParams(e) {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(e.ReasoningProtocol), reasoningProtocolNone)
}

// SetThinkingParams pins or releases the plain-chat request shape. Releasing it
// clears only the pin: an explicit protocol the user chose stays, so this never
// silently rewrites a deliberate deepseek/glm/kimi-k3 selection.
func SetThinkingParams(e *ProviderEntry, send bool) {
	if e == nil {
		return
	}
	if !send {
		e.ReasoningProtocol = reasoningProtocolNone
		return
	}
	if strings.EqualFold(strings.TrimSpace(e.ReasoningProtocol), reasoningProtocolNone) {
		e.ReasoningProtocol = ""
	}
}
