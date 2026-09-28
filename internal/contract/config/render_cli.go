package config

import (
	"fmt"
	"strings"
)

// renderCLIConfig writes the [cli] section, which is user-global only: a
// repo-local reasonix.toml must never run an external diff formatter. The
// section is emitted only when set, so the retired update_channel key never
// reappears.
func renderCLIConfig(b *strings.Builder, c *Config, scope RenderScope) {
	if scope == RenderScopeProject {
		return
	}
	if cmd := strings.TrimSpace(c.CLI.DiffFormatter); cmd != "" {
		b.WriteString("[cli]\n")
		fmt.Fprintf(b, "diff_formatter = %q   # external argv (no shell) formatting fenced diff blocks; the block is piped on stdin and its stdout is shown verbatim; user-global only\n", cmd)
		b.WriteString("\n")
	}
}
