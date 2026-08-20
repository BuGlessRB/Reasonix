package fileutil

import "strings"

// vcsStoreDirs are the directory names version-control systems keep their own
// store in. A store is not the work product — `git status` alone rewrites the
// index — so a change under one says nothing about whether the tree was
// touched, and nothing downstream may read it as evidence that it was.
var vcsStoreDirs = map[string]bool{".git": true, ".hg": true, ".svn": true}

// IsVCSStoreDir reports whether a single path element names a VCS store.
func IsVCSStoreDir(name string) bool { return vcsStoreDirs[name] }

// UnderVCSStore reports whether path is a VCS store or lies inside one.
func UnderVCSStore(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for part := range strings.SplitSeq(NormalizeSlashPath(path), "/") {
		if vcsStoreDirs[part] {
			return true
		}
	}
	return false
}
