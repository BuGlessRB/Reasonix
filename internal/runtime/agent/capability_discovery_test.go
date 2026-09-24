package agent

import (
	"strings"
	"testing"
)

// Every id the description names must be one the examples list checks.
func TestEveryNamedExampleIsReachable(t *testing.T) {
	desc := (*UseCapabilityTool)(nil).Description()
	for _, id := range CapabilityIDExamples {
		if !strings.Contains(desc, id) {
			t.Errorf("CapabilityIDExamples lists %q but the description never names it", id)
		}
	}
}
