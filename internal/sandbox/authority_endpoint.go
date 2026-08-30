//go:build !windows

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
)

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
