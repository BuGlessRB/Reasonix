package serve

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PagePrefix is the one path a built page owns. Everything else is the
// kernel's, so a route added to it later needs no change here.
const PagePrefix = "/_studio/"

// FindPage resolves the built page a host serves. An explicit directory holding
// none is a launch that would open on nothing, so it fails here rather than at
// the first paint. Nothing found is (nil, nil): the host serves the kernel
// alone, and says so in its own words.
func FindPage(dir string) (fs.FS, error) {
	if dir != "" {
		if !hasIndex(dir) {
			return nil, fmt.Errorf("no index.html under %s", dir)
		}
		return os.DirFS(dir), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(exe), "frontend-next", "dist"),
		filepath.Join("..", "frontend-next", "dist"),
		filepath.Join("desktop", "frontend-next", "dist"),
	} {
		if hasIndex(candidate) {
			return os.DirFS(candidate), nil
		}
	}
	return nil, nil
}

func hasIndex(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !st.IsDir()
}

// withPage puts the built page under a namespace of its own and leaves every
// other path to the kernel. The inverse — a list of the kernel's routes, with
// everything else falling through to the page — is what the asset server
// forced, and it has to be edited every time the kernel grows a route.
func withPage(kernel http.Handler, page fs.FS) http.Handler {
	if page == nil {
		return kernel
	}
	files := http.StripPrefix(PagePrefix, http.FileServer(http.FS(page)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, PagePrefix) {
			kernel.ServeHTTP(w, r)
			return
		}
		// A path inside the namespace that names no file is the page routing
		// itself, not a missing asset. Confined to the namespace, so it can
		// never answer for a route the kernel owns.
		name := strings.TrimPrefix(r.URL.Path, PagePrefix)
		if name != "" {
			if _, err := fs.Stat(page, strings.TrimSuffix(name, "/")); err != nil {
				http.ServeFileFS(w, r, page, "index.html")
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}
