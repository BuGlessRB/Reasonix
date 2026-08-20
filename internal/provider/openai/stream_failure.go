// How a body-phase failure is classified before it leaves the provider.
package openai

import (
	"errors"

	"reasonix/internal/provider"
)

// streamFailure decides whether a stream that stopped can be attempted again.
// A transport cut always can; a gateway reporting its own upstream failure can
// too, but only before it has said anything, since replaying after output
// would show it twice. The budget stays the Agent's — counting here too would
// stack two.
func streamFailure(emitted bool, err error) error {
	if provider.IsConnReset(err) {
		return provider.StreamInterrupt(err, provider.ClassifyStreamInterrupt(err))
	}
	var payload *provider.StreamPayloadError
	if !emitted && errors.As(err, &payload) {
		return provider.StreamInterrupt(err, provider.StreamInterruptUpstreamError)
	}
	return err
}
