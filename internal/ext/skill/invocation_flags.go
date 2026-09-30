package skill

import (
	"errors"
	"fmt"
	"strings"
)

// ErrModelInvocationDisabled marks a model-side call to a skill whose author
// declared it user-only (`disable-model-invocation: true`).
var ErrModelInvocationDisabled = errors.New("skill is user-invocable only")

// InvocationFlags is who may start a skill, as its author declared it.
type InvocationFlags struct {
	// DisableModelInvocation keeps the skill out of every model-facing surface
	// and refuses the model's own calls; only the user's /<name> reaches it.
	DisableModelInvocation bool
	// DisableUserInvocation hides the skill from the slash surface and refuses
	// a typed /<name>; the model still reaches it.
	DisableUserInvocation bool
	ArgumentHint          string // completion hint for /<name>
}

func parseInvocationFlags(fm map[string]string) InvocationFlags {
	return InvocationFlags{
		DisableModelInvocation: parseBoolFrontmatter(fm[skillFrontmatterDisableModel]),
		DisableUserInvocation:  isFalseFrontmatter(fm[skillFrontmatterUserInvocable]),
		ArgumentHint:           strings.TrimSpace(fm[skillFrontmatterArgumentHint]),
	}
}

func isFalseFrontmatter(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "false", "no", "0", "off":
		return true
	default:
		return false
	}
}

func (s Skill) modelCallError() error {
	if !s.DisableModelInvocation {
		return nil
	}
	return fmt.Errorf("%w: %q runs only when the user types /%s; do not call it yourself", ErrModelInvocationDisabled, s.Name, s.SlashName())
}

// ModelInvocable drops the skills whose author reserved them for the user.
func ModelInvocable(skills []Skill) []Skill {
	out := make([]Skill, 0, len(skills))
	for _, sk := range skills {
		if !sk.DisableModelInvocation {
			out = append(out, sk)
		}
	}
	return out
}
