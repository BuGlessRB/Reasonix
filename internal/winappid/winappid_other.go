//go:build !windows

package winappid

// SetProcessID is a no-op off Windows: taskbar grouping is owned by the
// platform shell there.
func SetProcessID() error {
	return nil
}

// EnsureShortcutIDs is a no-op off Windows.
func EnsureShortcutIDs() error {
	return nil
}
