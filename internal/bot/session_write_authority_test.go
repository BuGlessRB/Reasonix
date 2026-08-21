package bot

import (
	"testing"

	"reasonix/internal/control"
)

func mustBindBotControllerAuthority(t *testing.T, leases *control.SessionLeaseKeeper, ctrl *control.Controller) {
	t.Helper()
	if err := leases.BindControllerAuthority(ctrl); err != nil {
		t.Fatalf("bind controller authority: %v", err)
	}
}
