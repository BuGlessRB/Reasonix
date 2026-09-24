package anthropic

import "reasonix/internal/contract/provider"

type streamWireError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func anthropicStreamError(name string, wire *streamWireError) error {
	if wire == nil {
		return &provider.StreamPayloadError{Provider: name, Message: "stream error"}
	}
	return &provider.StreamPayloadError{Provider: name, Message: wire.Message, Type: wire.Type}
}
