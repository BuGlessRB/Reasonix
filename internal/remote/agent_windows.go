//go:build windows

package remote

import (
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// windowsAgentPipe is where Windows' own OpenSSH agent listens. The service
// fixes the name, so unlike Unix there is nothing to read it from.
const windowsAgentPipe = `\\.\pipe\openssh-ssh-agent`

// agentAuth offers what the Windows agent is holding. SSH_AUTH_SOCK wins where
// it is set: a port of the Unix agent puts its own pipe name there, and
// ignoring it would reach past a running agent to one that is not.
func agentAuth(identityFiles []string, identitiesOnly bool) (ssh.AuthMethod, func()) {
	pipe := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if pipe == "" {
		pipe = windowsAgentPipe
	}
	dial := func() (io.ReadWriteCloser, error) {
		// A named pipe opens as a file — CreateFile underneath, the same call
		// the agent's own clients make. A path that is not one fails the dial,
		// which costs this method rather than the connection.
		return os.OpenFile(pipe, os.O_RDWR, 0)
	}
	return agentAuthOver(dial, identityFiles, identitiesOnly)
}
