//go:build windows

package remote

import "golang.org/x/crypto/ssh"

// agentAuth reports ssh-agent as unavailable on Windows. The OpenSSH agent
// listens on the named pipe \\.\pipe\openssh-ssh-agent, and dialing one needs a
// Windows-specific transport that is a V2 follow-up; until then the caller
// falls back to key and password methods. Saying so here rather than failing
// inside a callback keeps the dead branch out of the shared path.
func agentAuth(_ []string, _ bool) (ssh.AuthMethod, func()) {
	return nil, func() {}
}
