//go:build linux

package main

import (
	"errors"
	"fmt"

	"reasonix/desktop/internal/update"
)

// Linux privileged-install error classes. errorClass maps these substrings into
// anonymous metrics buckets; authorization_cancelled is not recorded as a
// failure. The install itself lives in internal/update, keyed by this line.
var (
	errUpdateAuthCancelled = update.ErrDebAuthCancelled
	errUpdateAuthFailed    = update.ErrDebAuthFailed
	errUpdatePkgBusy       = update.ErrDebPkgBusy
	errUpdatePkgInstall    = update.ErrDebPkgInstall
	errUpdatePkgVerify     = update.ErrDebPkgVerify
	errUpdateCacheMismatch = errors.New("update: cached artifact does not match current install mode")
)

func applyDebLinux(packagePath, signaturePath string, onPhase func(phase string)) error {
	return desktopLine().InstallDeb(packagePath, signaturePath, onPhase)
}

func isAuthCancelled(err error) bool { return update.DebAuthCancelled(err) }

// ensureDebCacheMatchesProfile re-detects install mode at install time so a
// download made as portable cannot be applied after the install path changes.
func ensureDebCacheMatchesProfile(meta update.Cached, profile installProfile) error {
	switch profile.Mode {
	case installModeDeb:
		if meta.Kind != update.KindDeb {
			return errUpdateCacheMismatch
		}
		if meta.SignaturePath == "" {
			return errUpdateCacheMismatch
		}
	case installModePortable:
		if meta.Kind != update.KindTarball {
			return errUpdateCacheMismatch
		}
	default:
		return fmt.Errorf("update: install mode %s cannot self-update", profile.Mode)
	}
	return nil
}
