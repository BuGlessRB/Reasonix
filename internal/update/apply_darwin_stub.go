//go:build !darwin

package update

// MaybeRunMacHandoff is a no-op off macOS. Every host's main calls it before
// anything else so a re-executed handoff child never reaches normal startup;
// keeping the stub means no host has to know which platform it is compiled for.
func MaybeRunMacHandoff([]string) (handled bool, exitCode int) { return false, 0 }

// LocalApplication has nothing to state off macOS: every other platform
// replaces files rather than a bundle, so nothing reads what this returns. The
// stub is here for the same reason MaybeRunMacHandoff's is — a shell assembles
// one installer and does not branch on which platform it was compiled for.
func LocalApplication(Layout) (Application, error) { return Application{}, nil }
