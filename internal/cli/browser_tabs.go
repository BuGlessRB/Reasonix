package cli

import (
	"fmt"
	"strings"

	"reasonix/internal/browser"
)

// renderBrowserTabs lists the agent's open tabs, the active one marked.
func renderBrowserTabs(tabs []browser.TabInfo) string {
	if len(tabs) == 0 {
		return "the agent has no browser tabs open"
	}
	var b strings.Builder
	for _, t := range tabs {
		mark := " "
		if t.Active {
			mark = "*"
		}
		title := t.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(&b, "%s %s  %s  %s\n", mark, t.ID, title, t.URL)
	}
	return strings.TrimRight(b.String(), "\n")
}
