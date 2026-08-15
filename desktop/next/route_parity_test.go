package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Not every kernel route is reached through the typed client: the theme picker
// puts a pack's preview straight in an <img src>, so scanning sse.ts said the
// routing was fine while the picture rendered as a broken image. Reading the
// server's own registrations covers both kinds of caller at once.
func TestEveryRouteTheKernelRegistersIsRouted(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "internal", "serve", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no serve sources found: %v", err)
	}
	handler := regexp.MustCompile(`mux\.HandleFunc\("(?:GET|POST|PUT|PATCH|DELETE) (/[^"]*)", (s\.\w+)\)`)
	wildcard := regexp.MustCompile(`\{[^}]*\}`)
	seen := map[string]bool{}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range handler.FindAllStringSubmatch(string(src), -1) {
			route, served := m[1], m[2]
			if seen[route] {
				continue
			}
			seen[route] = true
			// s.index answers with the SPA itself, and /assets belongs to the
			// bundle the shell ships — routing either one to the kernel would
			// take the app's own entry point and stylesheet away from it.
			if served == "s.index" || strings.HasPrefix(route, "/assets/") {
				continue
			}
			probe := wildcard.ReplaceAllString(route, "x")
			if !isAPIPath(probe) {
				t.Errorf("%s registers %q but the shell routes %q to the assets, so it answers index.html", filepath.Base(file), route, probe)
			}
		}
	}
	if len(seen) < 40 {
		t.Fatalf("only found %d routes — the extraction broke, not the routing", len(seen))
	}
}
