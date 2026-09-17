package browser

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/fileutil"
)

// checkURL decides whether a page may be loaded. http and https go anywhere
// the network does; about:blank is the empty page; a file must lie inside one
// of roots once symlinks resolve. Every other scheme — the browser's own
// pages, script and data URLs — is refused.
func checkURL(raw string, roots []string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "", fail(CodeURLRefused, "%q is not an absolute URL; include the scheme, e.g. https://", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", fail(CodeURLRefused, "%q names no host", raw)
		}
		return u.String(), nil
	case "about":
		if u.Opaque == "blank" {
			return "about:blank", nil
		}
	case "file":
		path, err := url.PathUnescape(u.Path)
		if err != nil || !fileWithin(path, roots) {
			return "", fail(CodeURLRefused, "%q is outside the workspace; a page may only be a local file inside it", raw)
		}
		return u.String(), nil
	}
	return "", fail(CodeURLRefused, "the %s: scheme is not a page the agent may open", u.Scheme)
}

func fileWithin(path string, roots []string) bool {
	if path == "" {
		return false
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	path = filepath.FromSlash(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	for _, root := range roots {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		if fileutil.AtOrUnder(path, root) {
			return true
		}
	}
	return false
}

// OriginOf reduces a URL to what a site grant names. It answers "" for the
// empty page and anything that is not a page an agent may open.
func OriginOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return ""
		}
		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	case "file":
		return "file://"
	}
	return ""
}
