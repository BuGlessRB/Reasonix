package serve

import (
	"encoding/json"
	"net/http"
	"strings"

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

// skillEntry is one discoverable skill as a management surface needs it. The
// slash list answers "what can I type"; this answers "what may run", which is
// the larger set: a skill with no slash name still fires on model discovery.
type skillEntry struct {
	Name        string   `json:"name"`
	SlashName   string   `json:"slashName,omitempty"`
	Description string   `json:"description,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Plugin      string   `json:"plugin,omitempty"`
	Path        string   `json:"path,omitempty"`
	Subagent    bool     `json:"subagent,omitempty"`
	ReadOnly    bool     `json:"readOnly,omitempty"`
	Model       string   `json:"model,omitempty"`
	Effort      string   `json:"effort,omitempty"`
	AllowedURI  []string `json:"allowedTools,omitempty"`
	// Manual means the model never discovers it; only an explicit call runs it.
	Manual  bool `json:"manual,omitempty"`
	Enabled bool `json:"enabled"`
}

// skills lists every discoverable skill, disabled ones included, so the
// management surface can re-enable what it hid. Implicit reports whether
// model-initiated discovery is on at all — a global off makes every "auto"
// skill manual in practice, and hiding that would misreport all of them.
func (s *Server) skills(w http.ResponseWriter, r *http.Request) {
	ctl := s.ctl()
	raw := ctl.AllSkills()
	entries := make([]skillEntry, 0, len(raw))
	for _, sk := range raw {
		entries = append(entries, skillEntry{
			Name:        sk.Name,
			SlashName:   sk.SlashName(),
			Description: sk.Description,
			Scope:       string(sk.Scope),
			Plugin:      sk.Plugin,
			Path:        sk.Path,
			Subagent:    sk.RunAs == skill.RunSubagent,
			ReadOnly:    sk.ReadOnly,
			Model:       sk.Model,
			Effort:      sk.Effort,
			AllowedURI:  sk.AllowedTools,
			Manual:      strings.EqualFold(strings.TrimSpace(sk.Invocation), "manual"),
			Enabled:     ctl.SkillEnabled(sk.Name),
		})
	}
	writeJSON(w, map[string]any{
		"implicit": ctl.ImplicitSkillInvocationEnabled(),
		"skills":   entries,
	})
}

// skillEnabled persists one skill's enable switch. The runtime keeps serving the
// old prompt index until the session is rebuilt, so the response says so rather
// than letting the frontend imply the change already reached the model.
func (s *Server) skillEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	if err := s.ctl().SetSkillEnabled(strings.TrimSpace(body.Name), body.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"enabled": body.Enabled, "restartRequired": true})
}

// mcpEntry is one external tool provider. State is what the user can act on —
// running, still connecting, failed, disabled, or configured but not connected.
type mcpEntry struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Enabled   bool     `json:"enabled"`
	Transport string   `json:"transport,omitempty"`
	Source    string   `json:"source,omitempty"`
	Tools     int      `json:"tools"`
	Prompts   int      `json:"prompts,omitempty"`
	Resources int      `json:"resources,omitempty"`
	ToolNames []string `json:"toolNames,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// mcp lists the session's external tool providers. The activation switch is
// resolved for every configured name, because "off" and "never needed yet" look
// identical from the live host and mean opposite things to the user.
func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	ctl := s.ctl()
	out := []mcpEntry{}
	configured := ctl.ConfiguredMCPServers()
	enabled := make(map[string]bool, len(configured))
	source := make(map[string]string, len(configured))
	for _, st := range configured {
		enabled[st.Entry.Name] = st.Enabled
		source[st.Entry.Name] = string(st.Entry.Source)
	}
	// A runtime-only server has no configured declaration; it is live, so it is
	// on by definition.
	on := func(name string) bool {
		v, ok := enabled[name]
		return v || !ok
	}
	host := ctl.Host()
	seen := map[string]bool{}
	if host != nil {
		for _, srv := range host.Servers() {
			seen[srv.Name] = true
			names := make([]string, 0, len(srv.ToolList))
			for _, t := range srv.ToolList {
				names = append(names, t.Name)
			}
			out = append(out, mcpEntry{
				Name: srv.Name, State: "ready", Enabled: on(srv.Name),
				Transport: srv.Transport, Source: srv.ConfigSource,
				Tools: srv.Tools, Prompts: srv.Prompts, Resources: srv.Resources, ToolNames: names,
			})
		}
		for _, name := range host.ConnectingServers() {
			if !seen[name] {
				seen[name] = true
				out = append(out, mcpEntry{Name: name, State: "connecting", Enabled: on(name), Source: source[name]})
			}
		}
		for _, f := range host.Failures() {
			if seen[f.Name] {
				continue
			}
			seen[f.Name] = true
			out = append(out, mcpEntry{
				Name: f.Name, State: "failed", Enabled: on(f.Name),
				Transport: f.Transport, Source: source[f.Name], Error: f.Error,
			})
		}
	}
	// Configured but not live: switched off, or lazy and not needed yet. Saying
	// which one beats leaving them out of a list the user reads to check that
	// what they configured is actually there.
	for _, st := range configured {
		if seen[st.Entry.Name] {
			continue
		}
		state := "idle"
		if !st.Enabled {
			state = "disabled"
		}
		out = append(out, mcpEntry{
			Name: st.Entry.Name, State: state, Enabled: st.Enabled,
			Transport: st.Entry.Type, Source: string(st.Entry.Source),
		})
	}
	writeJSON(w, out)
}

// mcpReconnect retries one server. It answers with the refreshed row rather than
// a bare 204: the outcome the user is waiting for is the new state, and a
// follow-up GET would race the connect that just finished.
func (s *Server) mcpReconnect(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeMCPName(w, r)
	if !ok {
		return
	}
	tools, err := s.ctl().ReconnectMCPServer(name)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{"name": name, "state": "failed", "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"name": name, "state": "ready", "tools": tools})
}

// mcpEnabled flips the durable activation switch for one server.
func (s *Server) mcpEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if err := s.ctl().SetMCPServerEnabled(name, body.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"name": name, "enabled": body.Enabled})
}

func decodeMCPName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return "", false
	}
	return strings.TrimSpace(body.Name), true
}
