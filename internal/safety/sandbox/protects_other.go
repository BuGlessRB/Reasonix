//go:build !darwin && !windows

package sandbox

// confinedWriteDirs is every host directory bubblewrap binds writable for spec.
func confinedWriteDirs(spec Spec) []string {
	dirs := callerWriteDirs(spec)
	if !spec.MinimalWrites {
		dirs = append(dirs, linuxWriteDirs()...)
	}
	return dirs
}
