// apikeyenv.go — where a provider's key is stored, derived from its name.
package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// APIKeyEnvFor names the credential slot a provider's key lives in, derived
// from the provider name so two never share one. It sits beside isCredentialKey
// because building a name and judging one are the same rule: a second copy
// elsewhere produced "129_API_KEY" for a relay called "129", which the store
// then refused with nothing the person could act on.
func APIKeyEnvFor(name string) string {
	stem := strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, strings.ToUpper(strings.TrimSpace(name)))
	stem = strings.Trim(stem, "_")
	if stem == "" {
		return "CUSTOM_" + fnv1a32Hex(name) + "_API_KEY"
	}
	// An environment variable may not open with a digit, and a name that does
	// is ordinary for a self-hosted relay.
	if stem[0] >= '0' && stem[0] <= '9' {
		stem = "CUSTOM_" + stem
	}
	return stem + "_API_KEY"
}

func fnv1a32Hex(s string) string {
	hash := uint32(0x811c9dc5)
	for _, unit := range utf16.Encode([]rune(strings.TrimSpace(s))) {
		hash ^= uint32(unit)
		hash *= 0x01000193
	}
	return fmt.Sprintf("%08x", hash)
}

// ErrInvalidCredentialKey is refused when a slot name is not one an environment
// variable may carry. Callers tell it apart to say which field the person has
// to change: the name a slot is derived from, never the key they pasted.
var ErrInvalidCredentialKey = errors.New("credential slot name is not a usable environment variable")
