//go:build !linux

package main

import "fmt"

// installDebPackage is unreachable off Linux: nothing but dpkg owns an
// installed path, and OwnsInstalledPath is false everywhere else. It exists so
// the caller compiles.
func (a *App) installDebPackage(string, string, func(phase string)) error {
	return fmt.Errorf("update: package installs are Linux-only")
}
