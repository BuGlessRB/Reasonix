//go:build !windows

package main

// applyWindowsTaskbarIdentity is a no-op off Windows; taskbar grouping is
// owned by the platform shell there.
func applyWindowsTaskbarIdentity() {}
