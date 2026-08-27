// Package gitstatus reports what a working tree currently differs by, asking
// git rather than inferring it from what the agent did. The difference is the
// point: a file the agent created and a shell command then removed leaves two
// tool events behind and no change on disk, and only the tree can say so.
package gitstatus

import (
	"bytes"
	"context"
	"errors"
	"os"
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

// MaxDiffBytes caps one path's diff. A generated file can differ by megabytes,
// and a panel holding all of it is not being read — the cap is reported on the
// answer rather than applied behind the reader's back.
const MaxDiffBytes = 512 << 10

// ErrPathOutsideTree rejects a path that does not name something inside the
// tree. It is a sentinel because the caller has to tell it apart from git
// failing: one is a bad request, the other is a broken repository.
var ErrPathOutsideTree = errors.New("gitstatus: path is outside the working tree")

// safeRel constrains a caller-supplied path to the tree. Three things are being
// kept out: an absolute path, a traversal, and a leading dash — git reads that
// last one as an option however the argument list is built, which is why every
// invocation below also puts it after "--".
func safeRel(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || strings.HasPrefix(rel, "-") || filepath.IsAbs(rel) {
		return "", ErrPathOutsideTree
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	inside, err := filepath.Rel(root, filepath.Join(root, clean))
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", ErrPathOutsideTree
	}
	return filepath.ToSlash(clean), nil
}

// Diff returns the unified diff for one path in root's working tree, measured
// against HEAD so a staged change is included with an unstaged one. Untracked
// files are diffed against the null device instead: they have nothing in HEAD,
// and that is the only way git prints them without first writing to the index.
func Diff(ctx context.Context, root, path string) (text string, truncated bool, err error) {
	rel, err := safeRel(root, path)
	if err != nil {
		return "", false, err
	}
	// root goes through the dir parameter, not an "-C" argument: gitcmd hardens
	// only a subcommand it can see at args[0], and a diff against someone else's
	// repository is exactly the invocation those flags are for.
	var raw []byte
	if tracked(ctx, root, rel) {
		raw, err = gitcmd.Command(ctx, root, "diff", "--no-color", "HEAD", "--", rel).Output()
		if err != nil {
			// A repository with no commits has no HEAD to name; everything in
			// it is either staged or untracked.
			raw, err = gitcmd.Command(ctx, root, "diff", "--no-color", "--", rel).Output()
			if err != nil {
				return "", false, err
			}
		}
	} else {
		// --no-index exits 1 when the two sides differ, which is the whole
		// point of asking. Only the output matters here.
		raw, _ = gitcmd.Command(ctx, root, "diff", "--no-color", "--no-index", "--", os.DevNull, rel).Output()
	}
	if len(raw) > MaxDiffBytes {
		return string(raw[:MaxDiffBytes]), true, nil
	}
	return string(raw), false, nil
}

// tracked reports whether git already knows the path. The answer decides which
// of the two diffs above can say anything at all, so it is asked rather than
// inferred from an empty result — an unchanged tracked file and an untracked
// one both diff to nothing against HEAD.
func tracked(ctx context.Context, root, rel string) bool {
	return gitcmd.Command(ctx, root, "ls-files", "--error-unmatch", "--", rel).Run() == nil
}
