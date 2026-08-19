package control

import "reasonix/internal/config"

// displayPrefs is what a session shows, as opposed to what it does. Grouping
// them costs one field where loose ones cost one each, and the grouping is not
// cosmetic: independent flags multiply into states no type records as legal,
// which is what the struct-state ceiling exists to stop.
type displayPrefs struct {
	// Preferences a frontend sets mid-session; empty means follow the language
	// policy of the current user turn.
	responseLanguage  string
	reasoningLanguage string
}

func displayPrefsFrom(opts Options) displayPrefs {
	return displayPrefs{
		responseLanguage:  config.NormalizeLanguage(opts.ResponseLanguage),
		reasoningLanguage: config.NormalizeReasoningLanguage(opts.ReasoningLanguage),
	}
}
