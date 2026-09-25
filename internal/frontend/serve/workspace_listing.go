package serve

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// errListingOutsideTree is a folder whose real path leaves the workspace: a
// link inside the tree pointing out of it.
var errListingOutsideTree = errors.New("that folder is not inside the workspace")

// workspaceListing is what the explorer draws: files and folders, workspace-
// relative with forward slashes, each list sorted.
type workspaceListing struct {
	Files       []string `json:"files"`
	Directories []string `json:"directories"`
}

// listedName reports whether an entry belongs in the explorer: dot entries,
// VCS stores and dependency trees are left out, as the search leaves them.
func listedName(name string, dir bool) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	return !dir || (name != "node_modules" && name != "vendor")
}

// listFolder reads one folder of the workspace, rel being "" for the root.
// It reads that folder only: a sibling the process cannot open, or a tree
// too large to walk, has no bearing on opening this one.
func listFolder(root, rel string) (workspaceListing, error) {
	out := workspaceListing{Files: []string{}, Directories: []string{}}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if rel != "" {
		inside, err := insideRealRoot(root, dir)
		if err != nil {
			return out, err
		}
		if !inside {
			return out, errListingOutsideTree
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		if !listedName(entry.Name(), entry.IsDir()) {
			continue
		}
		path := entry.Name()
		if rel != "" {
			path = rel + "/" + path
		}
		if entry.IsDir() {
			out.Directories = append(out.Directories, path)
		} else {
			out.Files = append(out.Files, path)
		}
	}
	sort.Strings(out.Files)
	sort.Strings(out.Directories)
	return out, nil
}

// searchFiles walks the whole workspace for files whose path contains query.
// A folder it cannot read is passed over, so one unreadable corner does not
// turn every search into a failure.
func searchFiles(root, query string) (workspaceListing, error) {
	out := workspaceListing{Files: []string{}, Directories: []string{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		if !listedName(entry.Name(), entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || !filepath.IsLocal(rel) {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(strings.ToLower(rel), query) {
			out.Files = append(out.Files, rel)
		}
		return nil
	})
	sort.Strings(out.Files)
	return out, err
}

// insideRealRoot reports whether dir, links resolved, is still under root.
func insideRealRoot(root, dir string) (bool, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(realRoot, realDir)
	return err == nil && filepath.IsLocal(rel), nil
}
