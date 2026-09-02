package cli

import (
	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

func cliHistoryWithoutPinnedContextRevisions(messages []provider.Message) []provider.Message {
	for i, message := range messages {
		if !agent.IsPinnedContextRevision(message) {
			continue
		}
		visible := make([]provider.Message, 0, len(messages)-1)
		visible = append(visible, messages[:i]...)
		for _, candidate := range messages[i+1:] {
			if !agent.IsPinnedContextRevision(candidate) {
				visible = append(visible, candidate)
			}
		}
		return visible
	}
	return messages
}
