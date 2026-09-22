package cli

import (
	"context"
	"strings"

	"reasonix/internal/i18n"
)

// browserCommand opens a page in the session's browser. The terminal cannot
// draw it, but the tab is the same tab the agent reads and drives and the same
// one a window shows: one browser, whoever opened the page.
func (m *chatTUI) browserCommand(input string) {
	m.echoLocalCommand(input)
	address := strings.TrimSpace(strings.TrimPrefix(input, strings.Fields(input)[0]))
	if address == "" {
		m.notice("usage: /browser <url>")
		return
	}
	tab, err := m.ctrl.BrowserOpen(context.Background(), address, "", true)
	if err != nil {
		m.notice(i18n.M.ErrorPrefix + " " + err.Error())
		return
	}
	m.notice(tab.URL)
}
