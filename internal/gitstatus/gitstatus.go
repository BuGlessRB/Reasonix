// Package gitstatus reports what a working tree currently differs by, asking
// git rather than inferring it from what the agent did. The difference is the
// point: a file the agent created and a shell command then removed leaves two
// tool events behind and no change on disk, and only the tree can say so.
package gitstatus

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"

	"reasonix/internal/gitcmd"
)

// Change is one path git reports as differing from HEAD. Status is the
// porcelain XY code with surrounding space trimmed: "M", "A", "D", "R", "??".
type Change struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath,omitempty"`
	Status  string `json:"status"`
}

// Deleted reports whether the path is gone from the tree.
func (c Change) Deleted() bool { return strings.Contains(c.Status, "D") }

// Added reports whether the path is new — staged or still untracked.
func (c Change) Added() bool { return strings.Contains(c.Status, "A") || c.Status == "??" }

// Status lists the working tree's changes under root. A root that is not a git
// repository reports ok=false rather than an error: not every workspace is
// version-controlled, and a caller should fall back rather than show a failure.
func Status(ctx context.Context, root string) (changes []Change, ok bool, err error) {
	if strings.TrimSpace(root) == "" {
		return nil, false, nil
	}
	// Porcelain paths stay repository-relative even when -C points at a
	// subdirectory, and Windows spells one directory as both an 8.3 and a long
	// path — so take the prefix from git rather than from filepath.Rel.
	prefixRaw, err := gitcmd.Command(ctx, "", "-C", root, "rev-parse", "--show-prefix").Output()
	if err != nil {
		return nil, false, nil
	}
	prefix := strings.TrimSpace(string(prefixRaw))
	raw, err := gitcmd.Command(ctx, "", "-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".").Output()
	if err != nil {
		return nil, false, err
	}
	out := make([]Change, 0, 16)
	for _, c := range ParsePorcelainZ(raw) {
		c.Path = relFromPrefix(root, prefix, c.Path)
		if c.Path == "" {
			continue
		}
		c.OldPath = relFromPrefix(root, prefix, c.OldPath)
		out = append(out, c)
	}
	return out, true, nil
}

// ParsePorcelainZ decodes `git status --porcelain=v1 -z`. Rename and copy
// entries spend a second NUL-separated field on the source path.
func ParsePorcelainZ(raw []byte) []Change {
	parts := bytes.Split(raw, []byte{0})
	out := make([]Change, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if len(part) < 4 {
			continue
		}
		status := string(part[:2])
		entry := Change{Path: string(part[3:]), Status: strings.TrimSpace(status)}
		if strings.ContainsAny(status, "RC") && i+1 < len(parts) {
			i++
			entry.OldPath = string(parts[i])
		}
		out = append(out, entry)
	}
	return out
}

// RelPath normalises a status path to a slash-separated path inside base, or ""
// when it escapes base.
func RelPath(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(base, path); err == nil {
			path = rel
		}
	}
	path = filepath.Clean(path)
	if path == "." || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(path)
}

func relFromPrefix(base, prefix, path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	prefix = filepath.ToSlash(strings.TrimSpace(prefix))
	if path == "" {
		return ""
	}
	if prefix != "" {
		if !strings.HasPrefix(path, prefix) {
			return ""
		}
		path = strings.TrimPrefix(path, prefix)
	}
	return RelPath(base, filepath.FromSlash(path))
}
