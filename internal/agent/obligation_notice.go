// Telling the model what the host now owes, at the moment it changes. The text
// is a projection of the ledger's own answer, so a turn whose notice is dropped
// is still gated on the same debts — nothing here is load-bearing for
// correctness, and nothing downstream may treat it as the record.
package agent

import (
	"strings"

	"reasonix/internal/evidence"
)

// withObligationDelta appends what this call did to the host's debts. A call
// can settle one and create another at once — establishing a change's scope
// discharges the unproven mutation and stales every check before it — so both
// halves travel together rather than as two notices that could arrive apart.
func withObligationDelta(result string, delta evidence.ObligationDelta) string {
	if delta.Empty() {
		return result
	}
	var b strings.Builder
	b.WriteString("host obligations changed:")
	for _, o := range delta.Discharged {
		b.WriteString("\n- settled: " + string(o.Kind))
		if o.Cause != "" {
			b.WriteString(" (" + firstLine(o.Cause) + ")")
		}
	}
	for _, o := range delta.Added {
		b.WriteString("\n- owed: " + string(o.Kind))
		if o.Cause != "" {
			b.WriteString(" (" + firstLine(o.Cause) + ")")
		}
		if o.Discharge != "" {
			b.WriteString("\n  settled by: " + o.Discharge)
		}
	}
	if strings.TrimSpace(result) == "" {
		return b.String()
	}
	return strings.TrimRight(result, "\n") + "\n\n" + b.String()
}
