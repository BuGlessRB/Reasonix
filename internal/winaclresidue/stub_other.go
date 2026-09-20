//go:build !windows

package winaclresidue

import "errors"

// SweepStaleMarkers is a no-op outside Windows.
func SweepStaleMarkers() {}

// RepairLegacyCredentialDeny is a no-op outside Windows.
func RepairLegacyCredentialDeny(string) error { return nil }

// ResetCredentialDACL has no equivalent outside Windows.
func ResetCredentialDACL(string) error { return errors.ErrUnsupported }
