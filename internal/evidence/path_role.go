package evidence

import (
	"path/filepath"
	"strings"

	"reasonix/internal/fileutil"
)

// PathRole is what a changed path is to the work product. The zero value is
// production: a path no rule recognizes is code until something says otherwise,
// so an unfamiliar layout raises the risk it deserves instead of lowering it.
type PathRole uint8

const (
	// PathProduction is the work itself — what a reviewer has to look at.
	PathProduction PathRole = iota
	// PathSupporting ships with the work without being it: tests, fixtures,
	// documentation, localization, presentation.
	PathSupporting
	// PathVCSStore is the repository's own store, which is evidence of
	// nothing at any risk level (see fileutil.UnderVCSStore).
	PathVCSStore
)

// ClassifyPath is the single answer to what a changed path is. Risk scoring and
// review coverage both read it, so a path cannot be supporting material to one
// and production code to the other — they disagreed before, and the coverage
// side counted a repository store as code to be reviewed.
func ClassifyPath(path string) PathRole {
	if strings.TrimSpace(path) == "" {
		return PathSupporting
	}
	if fileutil.UnderVCSStore(path) {
		return PathVCSStore
	}
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	switch {
	case strings.HasSuffix(lower, "_test.go"), strings.HasSuffix(lower, "_test.ts"),
		strings.HasSuffix(lower, ".test.ts"), strings.HasSuffix(lower, ".test.tsx"),
		strings.HasSuffix(lower, "_spec.ts"), strings.HasSuffix(lower, ".spec.ts"):
		return PathSupporting
	case strings.Contains(lower, "/testdata/"), strings.Contains(lower, "/__tests__/"),
		strings.Contains(lower, "/fixtures/"):
		return PathSupporting
	case strings.HasSuffix(base, ".md"), strings.HasSuffix(base, ".mdx"),
		strings.HasSuffix(base, ".txt"), strings.HasSuffix(base, ".rst"):
		return PathSupporting
	case strings.Contains(lower, "/docs/"), strings.Contains(lower, "/locales/"),
		strings.Contains(lower, "/i18n/"), strings.HasPrefix(base, "readme"):
		return PathSupporting
	case strings.HasSuffix(base, ".css") && !strings.Contains(lower, "sandbox"):
		// Pure presentation styles, unless mixed with a sandbox path.
		return PathSupporting
	}
	return PathProduction
}
