package control

import (
	"context"
	"testing"
)

// Approving a site for the session answers for loading, reading and operating
// that site, and for no other site.
func TestApprovalSessionGrantScopesTheBrowserToOneOrigin(t *testing.T) {
	c, ids, prompts := approvalIDs()
	go func() {
		c.Approve(<-ids, true, true, false) // allow https://example.com for this session
		c.Approve(<-ids, true, false, false)
	}()

	calls := []struct{ tool, origin string }{
		{"browser_open", "https://example.com"},
		{"browser_act", "https://example.com"},
		{"browser_open", "https://example.com"},
		{"browser_act", "https://other.example"},
	}
	for i, call := range calls {
		allow, _, err := gateApprover{c}.Approve(context.Background(), call.tool, call.origin, nil)
		if err != nil || !allow {
			t.Fatalf("call %d = (%v,%v), want allow", i, allow, err)
		}
	}
	if *prompts != 2 {
		t.Errorf("prompted %d times, want 2 (one per origin)", *prompts)
	}
}
