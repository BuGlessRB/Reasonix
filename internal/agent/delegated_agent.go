// delegated_agent.go — the child a delegated run executes as.
package agent

import (
	"context"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// newDelegatedAgent builds that child, wiring what the call context decides
// rather than what a caller remembered to pass. The question surface is the
// case: a delegated run has a parent blocked on it and a person waiting, so one
// question can be outstanding — while fleet items, parallel tasks and
// background jobs carry no asker and stay silent here without needing a flag.
func newDelegatedAgent(ctx context.Context, prov provider.Provider, reg *tool.Registry,
	sess *Session, opts Options, sink event.Sink,
) *Agent {
	sub := New(prov, reg, sess, opts, sink)
	if _, _, asker, ok := CallContext(ctx); ok && asker != nil {
		sub.SetAsker(asker)
	}
	return sub
}
