package serve

import (
	"net/http"

	"reasonix/internal/skill"
)

// slashEntry is one thing the user can type after "/". Kind separates the two
// sources because they behave differently once invoked: a subagent skill runs
// in its own context and needs an argument, a command expands inline.
type slashEntry struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description,omitempty"`
	ArgHint     string `json:"argHint,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Plugin      string `json:"plugin,omitempty"`
	Subagent    bool   `json:"subagent,omitempty"`
}

// slash lists the complete slash surface Submit resolves, so a frontend can
// offer completion without restating what the controller accepts. Built-in
// verbs are absent on purpose: most of them are chat-TUI only, and offering
// one the HTTP path drops would be worse than not listing it.
func (s *Server) slash(w http.ResponseWriter, r *http.Request) {
	ctl := s.ctl()
	out := []slashEntry{}
	// Submit resolves a custom command before a skill of the same name; listing
	// them in that order keeps the menu's answer and the kernel's identical.
	seen := map[string]bool{}
	for _, c := range ctl.Commands() {
		if c.Hidden || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		out = append(out, slashEntry{
			Name: c.Name, Kind: "command", Description: c.Description,
			ArgHint: c.ArgHint, Plugin: c.Plugin,
		})
	}
	for _, sk := range ctl.SlashSkills() {
		name := sk.SlashName()
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, slashEntry{
			Name: name, Kind: "skill", Description: sk.Description,
			Scope: string(sk.Scope), Plugin: sk.Plugin,
			Subagent: sk.RunAs == skill.RunSubagent,
		})
	}
	writeJSONCached(w, r, out)
}

// mcpEntry is one external tool provider. State is what the user can act on —
// running, still connecting, failed, or configured but not connected — and it
// is the only field a healthy install ever changes.
type mcpEntry struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Transport string   `json:"transport,omitempty"`
	Source    string   `json:"source,omitempty"`
	Tools     int      `json:"tools"`
	Prompts   int      `json:"prompts,omitempty"`
	Resources int      `json:"resources,omitempty"`
	ToolNames []string `json:"toolNames,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// mcp lists the session's external tool providers. Read-only: connecting and
// removing servers rewrites user config, which needs its own surface.
func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	ctl := s.ctl()
	out := []mcpEntry{}
	host := ctl.Host()
	if host == nil {
		writeJSONCached(w, r, out)
		return
	}
	seen := map[string]bool{}
	for _, srv := range host.Servers() {
		seen[srv.Name] = true
		names := make([]string, 0, len(srv.ToolList))
		for _, t := range srv.ToolList {
			names = append(names, t.Name)
		}
		out = append(out, mcpEntry{
			Name: srv.Name, State: "ready", Transport: srv.Transport, Source: srv.ConfigSource,
			Tools: srv.Tools, Prompts: srv.Prompts, Resources: srv.Resources, ToolNames: names,
		})
	}
	for _, name := range host.ConnectingServers() {
		if !seen[name] {
			seen[name] = true
			out = append(out, mcpEntry{Name: name, State: "connecting"})
		}
	}
	for _, f := range host.Failures() {
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		out = append(out, mcpEntry{Name: f.Name, State: "failed", Transport: f.Transport, Error: f.Error})
	}
	// Configured but neither connected nor failed: lazy servers that have not
	// been needed yet. Saying "idle" beats leaving them out of a list the user
	// reads to check what they configured is actually there.
	for _, name := range ctl.ConfiguredMCPNames() {
		if !seen[name] {
			out = append(out, mcpEntry{Name: name, State: "idle"})
		}
	}
	writeJSONCached(w, r, out)
}
