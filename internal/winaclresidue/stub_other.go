//go:build !windows

package winaclresidue

// SweepStaleMarkers is a no-op outside Windows.
func SweepStaleMarkers() {}

// RepairLegacyCredentialDeny is a no-op outside Windows.
func RepairLegacyCredentialDeny(string) error { return nil }
