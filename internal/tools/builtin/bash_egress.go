package builtin

import (
	"io"
	"strings"

	"reasonix/internal/safety/egress"
	"reasonix/internal/safety/sandbox"
)

// routeEgress names a confined launch to the egress proxy and points its
// clients there. An escaped launch is not confined and gets neither.
func (b bash) routeEgress(prepared *sandbox.Prepared) {
	if !prepared.Wrapped || !b.sb.Network || b.sb.Egress == nil {
		return
	}
	prepared.EgressToken = egress.NewToken()
	prepared.EnvOverrides = append(prepared.EnvOverrides, b.sb.Egress.Env(prepared.EgressToken)...)
}

// egressNote is the host's account of what the egress proxy refused this
// launch. A download that failed on policy otherwise reaches the model as a
// bare connection error it has to guess the cause of.
func (b bash) egressNote(token string) string {
	if token == "" || b.sb.Egress == nil {
		return ""
	}
	refused := b.sb.Egress.Refusals(token)
	if len(refused) == 0 {
		return ""
	}
	return "[host] The sandbox refused network egress to: " + strings.Join(refused, ", ") +
		". Bash reaches only the hosts in [sandbox] allowed_domains; if the task needs one of these, ask the user to allow it."
}

func (b bash) writeEgressNote(out io.Writer, token string) {
	if note := b.egressNote(token); note != "" {
		_, _ = io.WriteString(out, "\n"+note+"\n")
	}
}
