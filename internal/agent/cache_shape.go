package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tokencount"
)

// PrefixShape hashes the portions of the request prefix that influence
// provider-side prompt-cache reuse. Comparing snapshots across turns
// lets us explain *why* a cache miss happened.
type PrefixShape struct {
	SystemHash        string
	ToolsHash         string
	PrefixHash        string
	LogRewriteVersion int
	ToolSchemaTokens  int
}

// CacheDiagnostics is a type alias for event.CacheDiagnostics so the agent
// can construct and compare diagnostics without importing event itself in
// every call site, while still assigning to event.Event.CacheDiagnostics.
type CacheDiagnostics = event.CacheDiagnostics

func shortHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:8])
}

// CaptureShape takes a snapshot of the current prefix state.
func CaptureShape(systemPrompt string, schemas []provider.ToolSchema, rewriteVersion int) PrefixShape {
	toolsJSON := NormalizedToolSchemas(schemas)
	return PrefixShape{
		SystemHash: shortHash(systemPrompt),
		ToolsHash:  shortHash(string(toolsJSON)),
		PrefixHash: shortHash(map[string]any{
			"system": systemPrompt,
			"tools":  string(toolsJSON),
		}),
		LogRewriteVersion: rewriteVersion,
		ToolSchemaTokens:  tokencount.Text(string(toolsJSON)),
	}
}

// NormalizedToolSchemas returns the exact JSON ToolsHash covers, so a recorder
// can persist the schema set a run sampled against and a reader can recompute
// the hash from it rather than trusting a second serialization.
func NormalizedToolSchemas(schemas []provider.ToolSchema) []byte {
	b, _ := json.Marshal(normalizeToolSchemas(schemas))
	return b
}

func normalizeToolSchemas(schemas []provider.ToolSchema) []provider.ToolSchema {
	out := make([]provider.ToolSchema, len(schemas))
	copy(out, schemas)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Description != out[j].Description {
			return out[i].Description < out[j].Description
		}
		return string(out[i].Parameters) < string(out[j].Parameters)
	})
	return out
}

// CompareShape returns diagnostics describing what changed between two shapes.
// contentReasons is the set of provider-visible rewrite reasons (e.g.
// "compact_auto", "snip", "rewind_truncate") drained from the Session since
// prev was captured — see Session.DrainContentRewriteReasons. It is the sole
// source of rewrite-caused reasons: a bare LogRewriteVersion change with no
// drained reason means only local-only metadata was touched (a decision
// receipt, tool-call preview/resolution, or an Edited-message replace), which
// never reaches the provider and so must not be reported as a cache change.
func CompareShape(prev, cur PrefixShape, usage *provider.Usage, contentReasons []string) CacheDiagnostics {
	reasons := []string{}
	if prev.SystemHash != "" && prev.SystemHash != cur.SystemHash {
		reasons = append(reasons, "system")
	}
	if prev.ToolsHash != "" && prev.ToolsHash != cur.ToolsHash {
		reasons = append(reasons, "tools")
	}
	reasons = append(reasons, contentReasons...)
	var miss, hit int
	if usage != nil {
		miss = usage.CacheMissTokens
		hit = usage.CacheHitTokens
	}
	return CacheDiagnostics{
		PrefixHash:          cur.PrefixHash,
		PrefixChanged:       len(reasons) > 0,
		PrefixChangeReasons: reasons,
		SystemHash:          cur.SystemHash,
		ToolsHash:           cur.ToolsHash,
		LogRewriteVersion:   cur.LogRewriteVersion,
		ToolSchemaTokens:    cur.ToolSchemaTokens,
		CacheMissTokens:     miss,
		CacheHitTokens:      hit,
	}
}

// ToolSchemaCost is a per-tool token cost estimate for diagnostic display.
type ToolSchemaCost struct {
	Name   string
	Tokens int
}
