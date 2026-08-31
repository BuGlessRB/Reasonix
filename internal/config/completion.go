package config

import (
	"fmt"
	"strings"
)

// Completion-validation modes, mirrored from the agent layer.
const (
	CompletionValidationOff     = "off"
	CompletionValidationShadow  = "shadow"
	CompletionValidationEnforce = "enforce"
)

// CompletionValidationMode returns the normalized completion-validation mode.
// Empty (unconfigured) defaults to shadow: the validator runs in record-only
// mode so behavior is unchanged while evidence accumulates.
func (a AgentConfig) CompletionValidationMode() string {
	switch strings.TrimSpace(a.CompletionValidation) {
	case CompletionValidationOff, CompletionValidationShadow, CompletionValidationEnforce:
		return strings.TrimSpace(a.CompletionValidation)
	default:
		return CompletionValidationShadow
	}
}

// ValidateCompletionValidation rejects an explicit invalid enum at load time
// rather than silently defaulting, so a typo surfaces where it is configured.
func ValidateCompletionValidation(value string) error {
	switch strings.TrimSpace(value) {
	case "", CompletionValidationOff, CompletionValidationShadow, CompletionValidationEnforce:
		return nil
	default:
		return fmt.Errorf("agent.completion_validation %q: must be off, shadow, or enforce", value)
	}
}

// renderCompletionValidation writes the full-config [agent] block lines for
// the completion validator.
func renderCompletionValidation(b *strings.Builder, c *Config) {
	if strings.TrimSpace(c.Agent.CompletionValidation) != "" {
		fmt.Fprintf(b, "completion_validation = %q   # off | shadow | enforce (empty defaults to shadow)\n", c.Agent.CompletionValidation)
	} else {
		b.WriteString("# completion_validation = \"shadow\"   # completion validator mode: off | shadow | enforce\n")
	}
	if strings.TrimSpace(c.Agent.CompletionEvaluatorModel) != "" {
		fmt.Fprintf(b, "completion_evaluator_model = %q   # optional; empty follows the working model\n", c.Agent.CompletionEvaluatorModel)
	} else {
		b.WriteString("# completion_evaluator_model = \"\"   # optional; empty follows the working model\n")
	}
}

// diffCompletionValidation appends the changed-field [agent] lines for the
// completion validator and reports whether anything was written.
func diffCompletionValidation(agentBuf *strings.Builder, c, d Config, anyAgent *bool) {
	if c.Agent.CompletionValidation != "" && c.Agent.CompletionValidation != d.Agent.CompletionValidation {
		fmt.Fprintf(agentBuf, "completion_validation = %q\n", c.Agent.CompletionValidation)
		*anyAgent = true
	}
	if c.Agent.CompletionEvaluatorModel != "" && c.Agent.CompletionEvaluatorModel != d.Agent.CompletionEvaluatorModel {
		fmt.Fprintf(agentBuf, "completion_evaluator_model = %q\n", c.Agent.CompletionEvaluatorModel)
		*anyAgent = true
	}
}
