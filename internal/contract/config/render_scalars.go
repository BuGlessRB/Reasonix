package config

import (
	"fmt"
	"strings"
)

// renderTopLevelScalars writes the bare top-level keys that precede every
// table, each annotated with its own comment. Project rendering omits the
// user-global credentials_store key.
func renderTopLevelScalars(b *strings.Builder, c *Config, scope RenderScope) {
	fmt.Fprintf(b, "config_version = %d   # schema marker for diagnostics; old versions may ignore it\n", configVersion(c))
	fmt.Fprintf(b, "default_model = %q\n", c.DefaultModel)
	if c.Language != "" {
		fmt.Fprintf(b, "language      = %q   # ui/model language; empty = auto-detect from $LANG / $REASONIX_LANG\n", c.Language)
	} else {
		b.WriteString("# language      = \"zh\"   # ui/model language; empty = auto-detect from $LANG / $REASONIX_LANG\n")
	}
	if scope != RenderScopeProject {
		if c.AutoSubmit {
			b.WriteString("auto_submit = true   # CLI ask: commit a multi-question batch once its last question is answered; user/global only\n")
		}
		fmt.Fprintf(b, "credentials_store = %q   # legacy compatibility; provider keys are saved in Reasonix's global .env\n", normalizeCredentialsStore(c.CredentialsStore))
	}
	b.WriteString("\n")
}
