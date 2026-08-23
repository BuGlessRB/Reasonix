package serve

import (
	"encoding/json"
	"net/http"
	"strings"

	"reasonix/internal/config"
)

// The host book is user-global: a machine is reachable from every project on
// this computer, so an entry written into one repository's file would follow
// that repository to someone who cannot reach it.

// RemoteHostEdit is one row of the book as the settings page sends it. Secrets
// are named, never carried: the two Env fields hold the name of a variable, the
// same shape a provider's key takes.
type RemoteHostEdit struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	IdentityFile  string `json:"identityFile"`
	ProxyJump     string `json:"proxyJump"`
	Workspace     string `json:"workspace"`
	ServeInstall  string `json:"serveInstall"`
	UseSSHConfig  bool   `json:"useSSHConfig"`
	PassphraseEnv string `json:"passphraseEnv"`
	PasswordEnv   string `json:"passwordEnv"`
}

func (h *Hub) saveRemoteHost(w http.ResponseWriter, r *http.Request) {
	if h.opts.Remote == nil {
		refuseNoRemote(w)
		return
	}
	var body RemoteHostEdit
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badBody(w)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		refuse(w, http.StatusBadRequest, "remote.name_required", "give this machine a name", nil)
		return
	}
	host := strings.TrimSpace(body.Host)
	if host == "" && !body.UseSSHConfig {
		// An entry that layers ssh_config has its address there; one that does
		// not has nowhere else to get it.
		refuse(w, http.StatusBadRequest, "remote.host_required", "say which address to dial", nil)
		return
	}
	if body.Port < 0 || body.Port > 65535 {
		refuse(w, http.StatusBadRequest, "remote.bad_port", "that is not a port number", nil)
		return
	}
	entry := config.RemoteHostEntry{
		Name:          name,
		Host:          host,
		Port:          body.Port,
		User:          strings.TrimSpace(body.User),
		IdentityFile:  strings.TrimSpace(body.IdentityFile),
		ProxyJump:     strings.TrimSpace(body.ProxyJump),
		Workspace:     strings.TrimSpace(body.Workspace),
		ServeInstall:  strings.TrimSpace(body.ServeInstall),
		UseSSHConfig:  body.UseSSHConfig,
		PassphraseEnv: strings.TrimSpace(body.PassphraseEnv),
		PasswordEnv:   strings.TrimSpace(body.PasswordEnv),
	}
	if host == "" {
		entry.Host = name
	}
	err := config.EditUserConfigWithCredentials(func(c *config.Config) ([]config.CredentialChange, error) {
		// Editing an entry must not drop the forwards it carries: they are set
		// from the CLI and have no control on this page to put them back.
		if existing, ok := c.RemoteHost(name); ok {
			entry.Forwards = append([]config.RemoteForwardEntry(nil), existing.Forwards...)
		}
		return nil, c.UpsertRemoteHost(entry)
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Hub) removeRemoteHost(w http.ResponseWriter, r *http.Request) {
	if h.opts.Remote == nil {
		refuseNoRemote(w)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badBody(w)
		return
	}
	name := strings.TrimSpace(body.Name)
	// A pane is still driving a kernel over there. Removing the entry would
	// leave a live link nothing in the book accounts for.
	if open := h.remotePanes(name); open > 0 {
		refuse(w, http.StatusConflict, "remote.has_open_panes",
			"close this machine's panes before removing it", map[string]any{"n": open})
		return
	}
	err := config.EditUserConfigWithCredentials(func(c *config.Config) ([]config.CredentialChange, error) {
		// Removing what is not there is the state the caller asked for, so it
		// is not an error to report.
		c.RemoveRemoteHost(name)
		return nil, nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// remoteCandidates are aliases in the user's ~/.ssh/config that the book does
// not have yet — the cheapest way to fill it on a machine already set up.
func (h *Hub) remoteCandidates(w http.ResponseWriter, _ *http.Request) {
	if h.opts.Remote == nil {
		refuseNoRemote(w)
		return
	}
	known := map[string]bool{}
	if cfg, err := config.Load(); err == nil {
		for _, entry := range cfg.Remote.Hosts {
			known[entry.Name] = true
		}
	}
	out := make([]string, 0)
	for _, alias := range h.opts.Remote.Candidates() {
		if !known[alias] {
			out = append(out, alias)
		}
	}
	writeJSON(w, out)
}

func (h *Hub) remotePanes(host string) int {
	n := 0
	for _, rt := range h.Runtimes() {
		if ep, ok := rt.Remote(); ok && ep.Host == host {
			n++
		}
	}
	return n
}

func refuseNoRemote(w http.ResponseWriter) {
	refuse(w, http.StatusNotImplemented, "remote.not_available",
		"this kernel does not open panes on other machines", nil)
}
