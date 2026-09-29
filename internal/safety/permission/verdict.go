package permission

import (
	"context"
	"encoding/json"
	"fmt"
)

// Refusal codes a Gate attaches to a call it refused. The code is the identity
// a caller reports and counts; the reason beside it is only the wording.
const (
	RefusalDenyRule   = "permission.deny_rule"
	RefusalReadOnly   = "permission.read_only"
	RefusalDeclined   = "permission.declined"
	RefusalUnattended = "permission.unattended"
)

// IsRefusalCode reports whether code is one a permission gate produces.
func IsRefusalCode(code string) bool {
	switch code {
	case RefusalDenyRule, RefusalReadOnly, RefusalDeclined, RefusalUnattended:
		return true
	}
	return false
}

// Verdict is a gate's answer to one call: whether it runs, and when it does
// not, the reason the model reads and the code a caller reports.
type Verdict struct {
	Allow  bool
	Reason string
	Code   string
}

// UnattendedApprover marks an approver that stands in for nobody. Its "no" is
// the absence of a person, which is a different fact from a person declining.
type UnattendedApprover interface {
	Unattended() bool
}

const readOnlyRefusal = "read-only mode: this session changes nothing — no file writes, no shell command that is not a known read, no tool that is not declared read-only. Do not retry it or rewrite it into another form; report what you would change, and the user can leave read-only mode to let it run."

// Check decides whether a tool call may run. It is the method the agent's Gate
// interface expects; Verdict is the same answer with the refusal's identity.
func (g *Gate) Check(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (bool, string, error) {
	v, err := g.Verdict(ctx, toolName, args, readOnly)
	return v.Allow, v.Reason, err
}

// DeniesWriters reports whether this gate refuses every call that is not a
// read, which a caller holding a path around the gate has to honour too.
func (g *Gate) DeniesWriters() bool { return g.Policy.ReadOnly }

// Verdict decides whether a tool call may run.
func (g *Gate) Verdict(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (Verdict, error) {
	if toolName == "bash" && !readOnly && BashCommandIsReadOnly(args) {
		readOnly = true
	}
	// Producing an install plan reads the source and writes nothing. Treating
	// the preview as the write it precedes would train the user to approve the
	// question that carries no information, ahead of the one that does.
	if toolName == "install_source" && !readOnly && InstallSourceIsPlanOnly(args) {
		readOnly = true
	}
	if g.Policy.ReadOnly && !readOnly && !g.Policy.ExplicitlyDenies(toolName, args) {
		return Verdict{Reason: readOnlyRefusal, Code: RefusalReadOnly}, nil
	}
	decision := g.Policy.Decide(toolName, readOnly, args)
	ruleReason := ""
	if rule, ok := g.Policy.MatchedRule(toolName, decision, args); ok {
		ruleReason = fmt.Sprintf("Matched permission rule: %s %s", decision, rule)
	}
	switch decision {
	case Deny:
		reason := "denied by permission policy — this tool/command is on the deny list. Do not retry it; choose another approach or stop and explain."
		if ruleReason != "" {
			reason = ruleReason + "\n" + reason
		}
		return Verdict{Reason: reason, Code: RefusalDenyRule}, nil
	case Ask:
		return g.ask(ctx, toolName, args, ruleReason)
	default:
		return Verdict{Allow: true}, nil
	}
}

func (g *Gate) ask(ctx context.Context, toolName string, args json.RawMessage, ruleReason string) (Verdict, error) {
	if g.Approver == nil {
		allow, reason, err := unattendedAsk(toolName, args)
		if !allow {
			return Verdict{Reason: reason, Code: RefusalUnattended}, err
		}
		return Verdict{Allow: true}, err
	}
	allow, remember, approverReason, err := g.approve(ctx, toolName, Subject(args), args, ruleReason)
	if err != nil {
		return Verdict{Reason: "approval aborted"}, err
	}
	if !allow {
		v := Verdict{Reason: "the user declined this tool call — do not retry it; ask how they would like to proceed or choose another approach.", Code: RefusalDeclined}
		if approverReason != "" {
			v.Reason = approverReason
		}
		if u, ok := g.Approver.(UnattendedApprover); ok && u.Unattended() {
			v.Code = RefusalUnattended
		}
		return v, nil
	}
	if remember && g.OnRemember != nil {
		// "Always allow" is tool-wide and deny rules still win on every call.
		// The rule also joins this Policy so a direct Decide sees it before
		// the next controller build.
		g.OnRemember(toolName)
		if rule, ok := ParseRule(toolName); ok {
			g.Policy.Allow = append(g.Policy.Allow, rule)
		}
	}
	return Verdict{Allow: true}, nil
}
