//go:build linux

package main

// installDebPackage hands a verified .deb to Polkit. Split by build tag for the
// reason applyDownloaded is: off Linux this call reaches a stub that always
// errors, and a runtime branch made every other platform compile a comparison
// whose answer was fixed.
func (a *App) installDebPackage(packagePath, signaturePath string, onPhase func(phase string)) error {
	return studioLine().InstallDeb(packagePath, signaturePath, onPhase)
}
