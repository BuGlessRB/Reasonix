// tool_facts.go — what a receipt is told about the tool behind a call.
package agent

import (
	"reasonix/internal/evidence"
	"reasonix/internal/tool"
)

// toolFacts reads a resolved tool's own contracts. Callers holding the tool use
// this; a replay that has only the name goes through Agent.toolFactsFor.
func toolFacts(t tool.Tool) evidence.ToolFacts {
	if t == nil {
		return readOnlyFacts
	}
	return evidence.ToolFacts{ReadOnly: t.ReadOnly(), WritesNamedPaths: tool.WritesNamedPaths(t)}
}

// readOnlyFacts is what an unresolvable call is credited with. A replay of a
// tool this build no longer registers must not invent a write.
var readOnlyFacts = evidence.ToolFacts{ReadOnly: true}

// facts is what a receipt is told about an executed call: the read-only verdict
// the host already settled — which for bash comes from parsing the command, not
// from the schema — and whether the tool that ran writes the paths it names.
func (p *toolCallPlan) facts() evidence.ToolFacts {
	return evidence.ToolFacts{ReadOnly: p.readOnly, WritesNamedPaths: tool.WritesNamedPaths(p.execTool)}
}

// toolFactsFor looks the name up in the live registry, which is the same route
// toolIsReadOnly takes for fold coverage.
func (a *Agent) toolFactsFor(name string) evidence.ToolFacts {
	if a == nil || a.svc.tools == nil {
		return readOnlyFacts
	}
	t, ok := a.svc.tools.Get(name)
	if !ok {
		return readOnlyFacts
	}
	return toolFacts(t)
}
