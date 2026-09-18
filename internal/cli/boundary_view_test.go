package cli

import (
	"strings"
	"testing"

	"reasonix/internal/control"
)

type grantingCtrl struct {
	control.SessionAPI
	granted []string
	revoked []string
}

func (c *grantingCtrl) PermissionRules() control.PermissionRules {
	return control.PermissionRules{Granted: append([]string(nil), c.granted...)}
}

func (c *grantingCtrl) RevokeSessionGrant(rule string) int {
	c.revoked = append(c.revoked, rule)
	if rule == "" {
		n := len(c.granted)
		c.granted = nil
		return n
	}
	for i, g := range c.granted {
		if g == rule {
			c.granted = append(c.granted[:i], c.granted[i+1:]...)
			return 1
		}
	}
	return 0
}

// What a prompt allowed for this session is in no file, so the boundary a
// person asks for has to say it, and there has to be a way back from it that
// is not ending the session.
func TestTheBoundaryListsSessionGrantsAndTakesThemBack(t *testing.T) {
	ctrl := &grantingCtrl{granted: []string{"Computer=com.example.Notes", "Browser=https://example.com"}}
	var commit []string
	m := &chatTUI{ctrl: ctrl, pendingCommit: &commit, width: 80}
	said := func() string { return strings.Join(commit, "\n") }

	m.showBoundary("/sandbox")
	if !strings.Contains(said(), "Computer=com.example.Notes") {
		t.Fatalf("the boundary did not name what this session allows: %q", said())
	}

	// `revoke` with nothing to revoke names what there is instead of saying
	// there is nothing.
	commit = nil
	m.showBoundary("/sandbox revoke")
	if !strings.Contains(said(), "Computer=com.example.Notes") || len(ctrl.revoked) != 0 {
		t.Fatalf("a bare revoke said %q and asked the kernel %v", said(), ctrl.revoked)
	}

	m.showBoundary("/sandbox revoke Computer=com.example.Notes")
	if len(ctrl.revoked) != 1 || ctrl.revoked[0] != "Computer=com.example.Notes" {
		t.Fatalf("revoking one asked for %v", ctrl.revoked)
	}
	m.showBoundary("/sandbox revoke all")
	if len(ctrl.revoked) != 2 || ctrl.revoked[1] != "" {
		t.Fatalf("revoking all asked for %v", ctrl.revoked)
	}
	// A rule nobody holds is said so rather than reported as taken back.
	commit = nil
	m.showBoundary("/sandbox revoke Computer=com.example.Gone")
	if !strings.Contains(said(), "com.example.Gone") {
		t.Fatalf("an unknown grant said %q", said())
	}
	if len(ctrl.revoked) != 2 {
		t.Fatalf("an unknown grant still asked the kernel: %v", ctrl.revoked)
	}
}
