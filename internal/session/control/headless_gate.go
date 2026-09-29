package control

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"reasonix/internal/safety/permission"
)

// SharedHeadlessGate is the one gate every headless-only sub-agent surface
// (task, writer-capable skills, the planner) holds. They capture it once with
// no rebuild hook, so a posture switch reaches them only through Update here;
// otherwise they stay on whatever mode was active when they were built.
type SharedHeadlessGate struct {
	mu     sync.RWMutex
	policy permission.Policy
	gate   *freshHumanHeadlessGate
}

// NewSharedHeadlessGate builds a shared gate holder from the base policy and
// the initial approval mode (see BuildHeadlessApprovalGate for the mode
// contract).
func NewSharedHeadlessGate(policy permission.Policy, mode string) *SharedHeadlessGate {
	g := &SharedHeadlessGate{policy: policy}
	g.Update(mode)
	return g
}

// Update rebuilds the held gate for a new approval mode. Safe to call
// concurrently with Check (a turn may be mid-flight on another goroutine when
// the user switches modes).
func (g *SharedHeadlessGate) Update(mode string) {
	next := BuildHeadlessApprovalGate(g.policy, mode)
	g.mu.Lock()
	g.gate = next
	g.mu.Unlock()
}

// RuleAllows reports whether a configured allow rule already covers this call.
// The fallback mode deliberately does not count: "auto approves writers" is a
// posture, while a matched rule is the user's own answer written down, and only
// the second one may stand in for asking again.
func (g *SharedHeadlessGate) RuleAllows(toolName string, args json.RawMessage, readOnly bool) bool {
	// Neutralizing the writer fallback is what separates the two: with Mode set
	// to Ask, an Allow can only have come from a rule that matched. Reading the
	// rule list directly would have to re-implement bash's segment matching.
	probe := g.policy
	probe.Mode = permission.Ask
	return probe.Decide(toolName, readOnly, args) == permission.Allow
}

func (g *SharedHeadlessGate) Check(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (bool, string, error) {
	return g.current().Check(ctx, toolName, args, readOnly)
}

// Verdict is Check with the refusal's identity.
func (g *SharedHeadlessGate) Verdict(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (permission.Verdict, error) {
	return g.current().Verdict(ctx, toolName, args, readOnly)
}

// DeniesWriters reports whether the current posture is read-only.
func (g *SharedHeadlessGate) DeniesWriters() bool { return g.current().DeniesWriters() }

func (g *SharedHeadlessGate) current() *freshHumanHeadlessGate {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.gate
}

func (g *SharedHeadlessGate) ExplicitlyDenies(toolName string, args json.RawMessage) bool {
	g.mu.RLock()
	gate := g.gate
	g.mu.RUnlock()
	return gate.ExplicitlyDenies(toolName, args)
}

type freshHumanHeadlessGate struct {
	gate                    *permission.Gate
	dynamicBashBypass       bool
	allowLowRiskFreshAction func(toolName string, args json.RawMessage) bool
}

func (g *freshHumanHeadlessGate) Check(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (bool, string, error) {
	v, err := g.Verdict(ctx, toolName, args, readOnly)
	return v.Allow, v.Reason, err
}

// Verdict is Check with the refusal's identity. A read-only posture answers
// first, so no narrower refusal below it can stand in for the real cause.
func (g *freshHumanHeadlessGate) Verdict(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (permission.Verdict, error) {
	if g.gate.DeniesWriters() {
		return g.gate.Verdict(ctx, toolName, args, readOnly)
	}
	if RequiresFreshHumanApprovalTool(toolName) {
		if !g.gate.ExplicitlyDenies(toolName, args) &&
			g.allowLowRiskFreshAction != nil &&
			g.allowLowRiskFreshAction(toolName, args) {
			return permission.Verdict{Allow: true}, nil
		}
		return permission.Verdict{Reason: "this tool requires fresh human approval and cannot run in a non-interactive session. Use an interactive session or a user-initiated memory command.", Code: permission.RefusalUnattended}, nil
	}
	if strings.EqualFold(toolName, "bash") {
		if blocker := permission.BashSubjectApprovalBlocker(permission.Subject(args)); blocker != permission.BashApprovalBlockerNone {
			if g.gate.Policy.Decide(toolName, readOnly, args) != permission.Allow && !g.dynamicBashBypass {
				return permission.Verdict{Reason: headlessBashBlockReason(blocker), Code: permission.RefusalUnattended}, nil
			}
		}
	}
	return g.gate.Verdict(ctx, toolName, args, readOnly)
}

// DeniesWriters reports a read-only posture.
func (g *freshHumanHeadlessGate) DeniesWriters() bool { return g.gate.DeniesWriters() }

func (g *freshHumanHeadlessGate) ExplicitlyDenies(toolName string, args json.RawMessage) bool {
	return g.gate.Policy.ExplicitlyDenies(toolName, args)
}

// Every blocked shape leads callers toward writing a script, so the writable
// boundary belongs in all of them: told only to use a file, models reach for
// /tmp and lose a second round to the sandbox.
const headlessBashBlockSuffix = " Scratch files belong under $TMPDIR, the session-private directory the host provides; a literal /tmp path is not writable, and anything else must be inside the workspace. The user can also switch to an interactive session or YOLO mode."

// headlessBashBlockReason names the shape that actually stopped the command.
// A caller told "inline interpreter code is blocked" about `env | grep` learns
// nothing and rewrites toward the wrong fix.
func headlessBashBlockReason(blocker permission.BashApprovalBlocker) string {
	const lead = "this shell command requires human approval and cannot run in a non-interactive session. "
	switch blocker {
	case permission.BashApprovalBlockerInlineCode:
		return lead + "It carries inline interpreter code (python -c, node -e, bash -c), which the host cannot audit; write the code to a file with write_file and run that file instead (e.g. write repro.py, then `python3 repro.py`)." + headlessBashBlockSuffix
	case permission.BashApprovalBlockerNestedExecution:
		return lead + "It nests another command through substitution ($(...), `...`, or <(...)), so what would actually run is not visible in the command; split it into separate calls and pass the values literally." + headlessBashBlockSuffix
	case permission.BashApprovalBlockerDynamicName:
		return lead + "The program it runs comes from a variable, so the host cannot tell what would execute; name the program literally." + headlessBashBlockSuffix
	case permission.BashApprovalBlockerIndirectExecution:
		return lead + "It runs through a wrapper (eval, source, xargs, env without a command, find -exec) that executes something the command itself does not name; call the target program directly." + headlessBashBlockSuffix
	case permission.BashApprovalBlockerHereDocBody:
		return lead + "It feeds a here-document, whose body is file content rather than arguments the host can read; write the file with write_file, then run whatever needs it." + headlessBashBlockSuffix
	default:
		return lead + "The host could not statically read what it would run; use a simpler literal command, or read_file/grep for inspection." + headlessBashBlockSuffix
	}
}
