package main

import "testing"

// The shell decides per request whether a path is the kernel's or the SPA's. A
// kernel path that falls through answers with index.html, and the frontend then
// fails parsing HTML as JSON — a failure that reads like a broken endpoint.
func TestIsAPIPathCoversResourceFamilies(t *testing.T) {
	for _, p := range []string{
		"/status", "/mcp", "/mcp/", "/mcp/reconnect", "/mcp/enabled",
		"/skills", "/skills/enabled", "/inbox/items", "/workspaces",
	} {
		if !isAPIPath(p) {
			t.Errorf("isAPIPath(%q) = false, want true", p)
		}
	}
	// Anything not claimed here has to reach the assets, or a deep link stops
	// rendering the app.
	for _, p := range []string{"/", "/sessions/abc", "/assets/app.js", "/mcpx", "/skillset"} {
		if isAPIPath(p) {
			t.Errorf("isAPIPath(%q) = true, want false", p)
		}
	}
}
