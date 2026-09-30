package boot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func disabledToolsMCPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "filtered", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{
				{"name": "read", "description": "Read fixture data.", "inputSchema": map[string]any{"type": "object"}, "annotations": map[string]any{"readOnlyHint": true}},
				{"name": "write", "description": "WRITE_SCHEMA_MARKER", "inputSchema": map[string]any{"type": "object"}},
			}}
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "error": map[string]any{"code": -32601, "message": "method not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
}

func TestEffectDisabledMCPToolsStayOutOfTheProviderAndCatalogAfterRestart(t *testing.T) {
	var rec *capabilityCallProvider
	provider.Register("boot-disabled-mcp", func(provider.Config) (provider.Provider, error) { return rec, nil })
	for _, load := range []string{"always", "deferred"} {
		t.Run(load, func(t *testing.T) {
			home := isolateConfigHome(t)
			t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
			workspace := robustTempDir(t)
			t.Chdir(workspace)
			server := disabledToolsMCPServer(t)
			defer server.Close()
			writeFile(t, workspace, "reasonix.toml", fmt.Sprintf(`
default_model = "test-model"
[agent]
system_prompt = "BASE"
[codegraph]
enabled = false
[[providers]]
name = "test-model"
kind = "boot-disabled-mcp"
model = "x"
[[plugins]]
name = "filtered"
type = "http"
url = %q
load = %q
disabled_tools = ["write"]
`, server.URL, load))
			approveWorkspace(t, workspace)
			approveProjectServer(t, workspace, "filtered")
			for _, phase := range []string{"fresh", "cached restart"} {
				rec = &capabilityCallProvider{call: `{"action":"inspect","capability_id":"mcp-server:filtered"}`}
				ctrl, err := Build(t.Context(), Options{Sink: event.Discard})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(ctrl.Close)
				if _, err := ctrl.ConnectMCPServer(ctrl.ConfiguredMCPServers()[0].Entry); err != nil {
					t.Fatal(err)
				}
				tools, err := ctrl.Host().ToolsFor(t.Context(), "filtered")
				if err != nil {
					t.Fatal(err)
				}
				if len(tools) != 1 || tools[0].Name() != "mcp__filtered__read" {
					t.Fatalf("%s: live tools = %v; want only read", phase, tools)
				}
				if got := ctrl.MCPCatalogTools()["filtered"]; got != 1 {
					t.Fatalf("%s: catalog tools=%d, want 1", phase, got)
				}
				if err := ctrl.Run(t.Context(), "describe available tools"); err != nil {
					t.Fatal(err)
				}
				requests := rec.requests()
				if len(requests) == 0 {
					t.Fatal("no provider request")
				}
				for _, req := range requests {
					body, err := json.Marshal(req)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(body), "mcp__filtered__write") || strings.Contains(string(body), "WRITE_SCHEMA_MARKER") {
						t.Fatalf("%s: disabled tool reached provider", phase)
					}
					if phase == "cached restart" && load == "always" && !slices.Contains(toolSchemaNames(req.Tools), "mcp__filtered__read") {
						t.Fatalf("%s: allowed tool missing from always-loaded provider schema", phase)
					}
				}
				results := effectToolResults(requests[len(requests)-1])
				if len(results) != 1 || !strings.Contains(results[0], "Read fixture data.") {
					t.Fatalf("%s: inspection did not return the allowed tool: %q", phase, results)
				}
				ctrl.Close()
			}
		})
	}
}
