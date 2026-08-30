package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// HostAuthority names a host service a confined command may call. It is a
// semantic endpoint, never a path: the backends below resolve it, so policy
// never has to spell /var/run/docker.sock and Windows can one day satisfy the
// same contract without owning a Unix socket.
type HostAuthority string

const (
	// SSHAgent signs with keys the sandbox cannot read. Possession of the key
	// file and the authority to use it are different things, which is why
	// denying the read (ForbidReadRoots) leaves this wide open.
	SSHAgent HostAuthority = "ssh_agent"
	Docker   HostAuthority = "docker"
	Podman   HostAuthority = "podman"
)

// GovernedAuthorities is what this package can actually enforce. Naming the set
// is what keeps the claim honest: host IPC at large is not isolated — DBus,
// Wayland, gpg-agent, keyrings and arbitrary sockets remain reachable — these
// endpoints are governed.
func GovernedAuthorities() []HostAuthority { return []HostAuthority{SSHAgent, Docker, Podman} }

// ParseAuthorities maps configured names onto the vocabulary. Unknown names are
// dropped rather than failing the session: a config written for a newer
// Reasonix must not make bash unusable, and an unknown name grants nothing.
func ParseAuthorities(names []string) []HostAuthority {
	governed := map[HostAuthority]bool{}
	for _, a := range GovernedAuthorities() {
		governed[a] = true
	}
	out := make([]HostAuthority, 0, len(names))
	for _, name := range names {
		if a := HostAuthority(strings.TrimSpace(name)); governed[a] {
			out = append(out, a)
		}
	}
	return out
}

// Granted reports whether the spec allows calling this authority. A governed
// authority absent from the list is masked, so the zero Spec denies every one
// of them and a new call site is confined until it says otherwise.
func (s Spec) Granted(a HostAuthority) bool { return slices.Contains(s.HostAuthorities, a) }

// deniedAuthorityEndpoints are the existing sockets a backend must mask.
func deniedAuthorityEndpoints(s Spec) []string {
	var out []string
	for _, a := range GovernedAuthorities() {
		if s.Granted(a) {
			continue
		}
		out = append(out, existingSockets(authorityEndpoints(a))...)
	}
	return out
}

// grantedAuthorityEndpoints are the sockets that must survive a blanket network
// denial, so revoking external egress does not silently revoke local signing.
func grantedAuthorityEndpoints(s Spec) []string {
	var out []string
	for _, a := range GovernedAuthorities() {
		if s.Granted(a) {
			out = append(out, existingSockets(authorityEndpoints(a))...)
		}
	}
	return out
}

// authorityEndpoints resolves an authority the way its own client resolves it,
// so a non-default daemon is governed rather than quietly ungoverned.
func authorityEndpoints(a HostAuthority) []string {
	switch a {
	case SSHAgent:
		return []string{os.Getenv("SSH_AUTH_SOCK")}
	case Docker:
		out := []string{unixHost(os.Getenv("DOCKER_HOST")), "/var/run/docker.sock"}
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out, filepath.Join(home, ".docker", "run", "docker.sock"))
		}
		return out
	case Podman:
		out := []string{unixHost(os.Getenv("CONTAINER_HOST")), "/run/podman/podman.sock"}
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			out = append(out, filepath.Join(dir, "podman", "podman.sock"))
		}
		return out
	}
	return nil
}

// unixHost extracts the path from a unix:// endpoint; a tcp:// daemon is
// reached over the network axis instead and is not a socket to mask.
func unixHost(v string) string {
	if path, ok := strings.CutPrefix(strings.TrimSpace(v), "unix://"); ok {
		return path
	}
	return ""
}

// existingSockets keeps the paths that are sockets right now, resolved through
// symlinks so the backends match the canonical path the kernel sees. A missing
// endpoint is dropped: there is nothing to mask, and a mount destination that
// does not exist fails an otherwise valid sandbox closed.
func existingSockets(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		info, err := os.Stat(abs)
		if err != nil || info.Mode()&os.ModeSocket == 0 || seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}
