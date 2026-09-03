package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"reasonix/internal/config"
)

// The page gets one namespace and the kernel keeps everything else. The inverse
// — a list of the kernel's routes, with the rest falling through to the page —
// is the arrangement this mount exists to stop repeating.
func TestPageIsServedInItsOwnNamespace(t *testing.T) {
	page := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<title>studio</title>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("// the page")},
	}
	kernel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"from":"the kernel"}`)
	})
	srv := httptest.NewServer(withPage(kernel, page))
	defer srv.Close()

	for _, c := range []struct{ path, want string }{
		{"/_studio/", "<title>studio</title>"},
		{"/_studio/assets/app.js", "// the page"},
		// Client-side routing inside the namespace, which is the page's own
		// business and can never answer for a route the kernel owns.
		{"/_studio/sessions/whatever", "<title>studio</title>"},
		{"/status", "the kernel"},
		{"/", "the kernel"},
		// The prefix is the whole segment: a path that merely starts with the
		// same letters belongs to the kernel like any other.
		{"/_studioish", "the kernel"},
	} {
		t.Run(c.path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + c.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), c.want) {
				t.Errorf("%s answered %q, want it to contain %q", c.path, body, c.want)
			}
		})
	}
}

// The page sits inside the auth gate. A window reaches it behind a loopback
// gate that answers first, but a serve bound to an address has only this one —
// and a page served ahead of it is one handed to whoever can reach the port.
func TestPageIsBehindTheAuthGate(t *testing.T) {
	hub := NewHub(HubOptions{
		Serve: config.ServeConfig{AuthMode: "token", Token: "the-token"},
		Page:  fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<title>studio</title>")}},
	})
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + PagePrefix)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("the page answered %d without a credential: %q", resp.StatusCode, body)
	}
}

// An explicit directory holding no page is a launch that would open on nothing,
// and it fails where the reader can still act on it rather than at first paint.
func TestFindPageRefusesADirectoryWithNoIndex(t *testing.T) {
	if _, err := FindPage(t.TempDir()); err == nil {
		t.Fatal("a directory with no index.html was accepted as a page")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>x</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := FindPage(dir)
	if err != nil || got == nil {
		t.Fatalf("FindPage(%q) = %v, %v; want the directory", dir, got, err)
	}
}
