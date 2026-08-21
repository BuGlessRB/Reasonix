// How a body-phase failure is classified before it leaves the provider.
package openai

import (
	"errors"
	"fmt"
	"io"
	"time"

	"reasonix/internal/provider"
)

// streamFailure decides whether a stream that stopped can be attempted again.
// A transport cut always can; a gateway reporting its own upstream failure can
// too, but only before it has said anything, since replaying after output
// would show it twice. The budget stays the Agent's — counting here too would
// stack two.
func streamFailure(emitted bool, err error) error {
	// A cut the read loop named already carries its reason; re-wrapping would
	// bury it under one this cannot infer as well.
	var interrupted *provider.StreamInterruptedError
	if errors.As(err, &interrupted) {
		return err
	}
	if provider.IsConnReset(err) {
		return provider.StreamInterrupt(err, provider.ClassifyStreamInterrupt(err))
	}
	var payload *provider.StreamPayloadError
	if !emitted && errors.As(err, &payload) {
		return provider.StreamInterrupt(err, provider.StreamInterruptUpstreamError)
	}
	return err
}

// stallCut is the end only this read loop can recognise: the deadline it set
// expired. Downstream the error is an unexpected EOF like any other, so the
// reason is attached here rather than inferred from the message later.
func stallCut(name string, idle time.Duration) error {
	err := fmt.Errorf("%s: stream stalled — no data for %s, connection likely dropped: %w", name, idle, io.ErrUnexpectedEOF)
	return provider.StreamInterrupt(err, provider.StreamInterruptIdleTimeout)
}

// prematureCut is a clean close before any terminal the protocol accepts.
func prematureCut(name string) error {
	err := fmt.Errorf("%s: stream ended before completion: %w", name, io.ErrUnexpectedEOF)
	return provider.StreamInterrupt(err, provider.StreamInterruptPrematureEOF)
}
