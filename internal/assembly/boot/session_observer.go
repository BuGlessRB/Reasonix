package boot

import (
	"reasonix/internal/runtime/agent"
	"reasonix/internal/state/history"
)

func newObservedSession(systemPrompt string) *agent.Session {
	session := agent.NewSession(systemPrompt)
	session.SetPersistObserver(history.PersistObserver())
	return session
}
