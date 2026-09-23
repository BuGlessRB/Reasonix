package cli

import (
	"fmt"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/i18n"
)

// mostRecentSession returns the chronologically newest saved session for
// --continue. Interactive resume surfaces deliberately group recovery families
// and prefer visible leaves, but --continue promises the most recent session and
// must not let that presentation ordering select an older recovery copy.
func mostRecentSession(dir string) (agent.SessionInfo, bool) {
	if dir == "" {
		return agent.SessionInfo{}, false
	}
	sessions, err := agent.ListSessions(dir)
	if err != nil || len(sessions) == 0 {
		return agent.SessionInfo{}, false
	}
	return sessions[0], true
}

func recoverySessionBadge(s agent.SessionInfo) string {
	if !s.Recovered {
		return ""
	}
	parent := strings.TrimSpace(s.ParentID)
	if len(parent) > 8 {
		parent = parent[:8]
	}
	if parent == "" {
		parent = "?"
	}
	return fmt.Sprintf(i18n.M.ResumeRecoveryBadgeFmt, parent) + " "
}
