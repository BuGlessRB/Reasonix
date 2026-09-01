package control

import (
	"errors"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

type controllerPromptState struct {
	base   string
	pinned string
}

func newControllerPromptState(base, pinned string, executor *agent.Agent) controllerPromptState {
	current := ""
	if executor != nil && executor.Session() != nil {
		messages := executor.Session().Snapshot()
		if len(messages) > 0 && messages[0].Role == provider.RoleSystem {
			current = messages[0].Content
			if base == "" {
				base = current
			}
		}
	}
	state := controllerPromptState{base: base, pinned: strings.TrimSpace(pinned)}
	if executor != nil && executor.Session() != nil && state.composed() != current {
		executor.Session().SetLeadingSystemPrompt(state.composed())
	}
	return state
}

func (s controllerPromptState) composed() string {
	if s.pinned == "" {
		return s.base
	}
	if s.base == "" {
		return s.pinned
	}
	return strings.TrimRight(s.base, "\n") + "\n\n" + s.pinned
}

// ApplyExtensionSystemPrompt replaces only the host/extension-owned base
// prompt. Session-owned pinned context remains a separate stable suffix.
func (c *Controller) ApplyExtensionSystemPrompt(prompt string) {
	if c == nil || c.executor == nil {
		return
	}
	c.mu.Lock()
	c.prompt.base = prompt
	composed := c.prompt.composed()
	c.mu.Unlock()
	c.executor.SetSession(agent.NewSession(composed))
}

func (c *Controller) basePrompt() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prompt.base
}

// SystemPrompt returns the current controller system prompt.
func (c *Controller) SystemPrompt() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prompt.composed()
}

// SetPinnedContext atomically changes the controller-owned prompt suffix and
// the active Session's leading system message. A running turn or concurrent
// session rotation is rejected so provider request assembly never observes a
// mixed prompt generation.
func (c *Controller) SetPinnedContext(pinned string) error {
	if c == nil {
		return nil
	}
	pinned = strings.TrimSpace(pinned)
	c.mu.Lock()
	unchanged := c.prompt.pinned == pinned
	c.mu.Unlock()
	if unchanged {
		return nil
	}
	if err := c.beginRotation(); err != nil {
		if errors.Is(err, errTurnRunningRotation) {
			return ErrTurnRunning
		}
		return err
	}
	defer c.endRotation()
	c.snapshotMu.Lock()
	defer c.snapshotMu.Unlock()
	c.mu.Lock()
	c.prompt.pinned = pinned
	composed := c.prompt.composed()
	exec := c.executor
	c.mu.Unlock()
	if exec != nil && exec.Session() != nil {
		exec.Session().SetLeadingSystemPrompt(composed)
	}
	return nil
}
