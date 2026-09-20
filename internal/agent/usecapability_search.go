package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/capability"
	"reasonix/internal/plugin"
	"reasonix/internal/retrieval"
)

type capabilitySearchHit struct {
	ID          string            `json:"id"`
	Aliases     []string          `json:"aliases,omitempty"`
	Kind        capability.Kind   `json:"kind"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Status      capability.Status `json:"status"`
	ReadOnly    bool              `json:"read_only,omitempty"`
	InputSchema json.RawMessage   `json:"input_schema,omitempty"`
}

func (t *UseCapabilityTool) listCapabilities() (string, error) {
	type capInfo struct {
		ID          string   `json:"id"`
		Aliases     []string `json:"aliases,omitempty"`
		Kind        string   `json:"kind"`
		Name        string   `json:"name"`
		Status      string   `json:"status,omitempty"`
		ReadOnly    bool     `json:"read_only,omitempty"`
		Description string   `json:"description,omitempty"`
	}
	var caps []capInfo
	if t.catalog != nil {
		for _, e := range t.catalog().Entries {
			if e.Kind == capability.KindTool && t.registry != nil && t.registry.ProviderVisible(e.ToolName) {
				continue
			}
			caps = append(caps, capInfo{
				ID: e.ID, Aliases: e.Aliases, Kind: string(e.Kind), Name: e.Name,
				Status: string(e.Status), ReadOnly: e.ReadOnly, Description: capabilityLead(e.Description),
			})
		}
	}
	serversJSON, err := t.listServers()
	if err != nil {
		return "", err
	}
	var serversPayload struct {
		Servers []listServerInfo `json:"servers"`
		Note    string           `json:"note"`
	}
	_ = json.Unmarshal([]byte(serversJSON), &serversPayload)
	payload := struct {
		Note         string           `json:"note"`
		Capabilities []capInfo        `json:"capabilities"`
		Servers      []listServerInfo `json:"servers"`
	}{
		Note:         "This is the full catalog and may be truncated in conversation. Prefer action=search with capability terms, then action=call with a returned id.",
		Capabilities: caps,
		Servers:      serversPayload.Servers,
	}
	if serversPayload.Note != "" {
		payload.Note += " " + serversPayload.Note
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *UseCapabilityTool) listServers() (string, error) {
	configured := t.configuredServers()
	list := make([]listServerInfo, 0, len(configured))
	for _, server := range configured {
		spec := server.spec
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		resolved := plugin.ResolveStoredAuthorization(context.Background(), spec)
		connected := server.enabled && resolved.ServerAuthorized() && t.host != nil && t.host.HasClientForSpec(resolved)
		status := "configured"
		if !server.enabled {
			status = "disabled"
		} else if connected {
			status = "ready"
		} else if t.host != nil {
			for _, f := range t.host.Failures() {
				if f.Name == name && strings.TrimSpace(f.Error) != "" {
					status = "failed"
					break
				}
			}
		}
		list = append(list, listServerInfo{
			Name: name, CapabilityID: "mcp-server:" + name, Status: status,
			Authorized: resolved.ServerAuthorized(), Connected: connected,
		})
	}
	b, err := json.MarshalIndent(map[string]any{
		"servers": list,
		"note":    "list does not start MCP servers. Call action=call on mcp-server:<name> to connect after authorization, or mcp-tool:<server>/<tool> for a concrete tool.",
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *UseCapabilityTool) searchCapabilities(query string, limit int) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query is required for action=search")
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	catalog := t.currentCatalog()
	docs := make([]retrieval.FieldedDoc, 0, len(catalog.Entries))
	entries := make(map[string]capability.Entry, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if entry.Status == capability.StatusDisabled || entry.Status == capability.StatusFailed {
			continue
		}
		fields := map[string]string{
			"name":        entry.ID + " " + entry.Name + " " + entry.ToolName,
			"description": entry.Description,
			"keywords":    strings.Join(entry.Aliases, " "),
		}
		if entry.Kind == capability.KindTool && t.registry != nil {
			if target, ok := t.registry.Get(entry.ToolName); ok {
				fields["body"] = string(target.Schema())
			}
		}
		docs = append(docs, retrieval.FieldedDoc{ID: entry.ID, Fields: fields})
		entries[entry.ID] = entry
	}

	ranked := retrieval.RankV2(query, docs)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	hits := make([]capabilitySearchHit, 0, len(ranked))
	for _, rankedHit := range ranked {
		entry, ok := entries[rankedHit.ID]
		if !ok {
			continue
		}
		hit := capabilitySearchHit{
			ID:          entry.ID,
			Aliases:     entry.Aliases,
			Kind:        entry.Kind,
			Name:        entry.Name,
			Description: capabilityLead(entry.Description),
			Status:      entry.Status,
			ReadOnly:    entry.ReadOnly,
		}
		if entry.Kind == capability.KindTool && t.registry != nil {
			if target, ok := t.registry.Get(entry.ToolName); ok {
				hit.InputSchema = target.Schema()
			}
		}
		hits = append(hits, hit)
	}

	payload := map[string]any{
		"query":   query,
		"matches": hits,
		"note":    "Call action=call with a returned capability id. An empty matches array means this catalog search found no lexical match; it does not prove that every possible external capability is unavailable.",
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func filterCapabilitySearchResult(raw string, allowed map[string]bool) string {
	var payload struct {
		Query   string                `json:"query"`
		Matches []capabilitySearchHit `json:"matches"`
		Note    string                `json:"note"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return `{"query":"","matches":[],"note":"search is filtered to this subagent's allowed capabilities."}`
	}
	filtered := payload.Matches[:0]
	for _, hit := range payload.Matches {
		if allowed[hit.ID] || anyAllowedAlias(hit.Aliases, allowed) {
			filtered = append(filtered, hit)
		}
	}
	payload.Matches = filtered
	payload.Note = "search is filtered to this subagent's allowed capabilities."
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return `{"query":"","matches":[],"note":"search is filtered to this subagent's allowed capabilities."}`
	}
	return string(b)
}

func anyAllowedAlias(aliases []string, allowed map[string]bool) bool {
	for _, alias := range aliases {
		if allowed[alias] {
			return true
		}
	}
	return false
}
