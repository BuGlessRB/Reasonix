// Package taskpolicy builds the host-side TaskPolicy that freezes verification
// and natural-language constraints for one turn before the first model request.
// It never calls a classification model.
//
// Two things bind a turn: the frozen role setting, and constraints the user
// stated outright ("don't modify", "only run go test"). How much review a
// change owes is decided after the fact by the mutation receipts it produced —
// see evidence.Ledger.MutationRiskAfter — not by anything read off the request.
package taskpolicy

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"reasonix/internal/agentpreset"
	"reasonix/internal/shellparse"
)

// PolicyVersion is the diagnostic version stamped on every TaskPolicy.
const PolicyVersion = agentpreset.PolicyVersion

// Verification is the verification level for this turn.
type Verification = agentpreset.VerificationLevel

const (
	VerifyNone     = agentpreset.VerifyNone
	VerifyTargeted = agentpreset.VerifyTargeted
	VerifyFull     = agentpreset.VerifyFull
)

// Constraints are natural-language and host-boundary limits for the turn.
type Constraints struct {
	// ForbidMutation blocks every real writer before execution.
	ForbidMutation bool
	// ForbidTests blocks verification commands; gaps become Partial.
	ForbidTests bool
	// AllowedChecks, when non-empty, limits verification to those exact commands.
	AllowedChecks []string
	// ForbidExternal blocks push/publish/deploy-style external actions.
	ForbidExternal bool
	// PlanModeReadOnly is the explicit plan-mode read-only boundary.
	PlanModeReadOnly bool
	// Notes records structured reasons for diagnostics (never user-facing).
	Notes []string
}

// Input is the host-trusted material used to derive a TaskPolicy. Quoted and
// fenced regions must already be stripped by StripQuotedConstraints or left
// intact so Derive ignores them.
type Input struct {
	// Raw is the original user text (including quotes/fences); the fallback
	// source when Instruction is empty.
	Raw string
	// Instruction is user text with quoted/fenced content removed so constraint
	// phrases inside citations cannot bind the host.
	Instruction string
	// Preset is the frozen role setting for this turn.
	Preset agentpreset.AgentPreset
	// PlanMode is the collaboration plan-mode flag.
	PlanMode bool
}

// TaskPolicy is the authoritative host policy for one turn.
type TaskPolicy struct {
	Preset       agentpreset.AgentPreset
	Constraints  Constraints
	Verification Verification
	// PolicyVersion is diagnostic only.
	PolicyVersion int
}

// Derive builds a TaskPolicy from host-trusted input without model calls.
func Derive(in Input) TaskPolicy {
	preset := agentpreset.Normalize(string(in.Preset))
	policy := agentpreset.PolicyOf(preset)

	instruction := strings.TrimSpace(in.Instruction)
	if instruction == "" {
		instruction = StripQuotedConstraints(in.Raw)
	}

	constraints := parseConstraints(instruction)
	if in.PlanMode {
		constraints.PlanModeReadOnly = true
		constraints.ForbidMutation = true
		constraints.Notes = append(constraints.Notes, "plan_mode_read_only")
	}

	if constraints.ForbidTests {
		// Host still records the gap as Partial; verification commands stay blocked.
		constraints.Notes = append(constraints.Notes, "forbid_tests")
	}

	return TaskPolicy{
		Preset:        preset,
		Constraints:   constraints,
		Verification:  policy.VerificationPolicy.Level,
		PolicyVersion: PolicyVersion,
	}
}

// AllowsMutation reports whether a real writer may proceed under this policy.
func (p TaskPolicy) AllowsMutation() bool {
	return !p.Constraints.ForbidMutation && !p.Constraints.PlanModeReadOnly
}

// AllowsTests reports whether verification commands may run.
func (p TaskPolicy) AllowsTests() bool {
	return !p.Constraints.ForbidTests
}

// AllowsCommand reports whether a specific verification command is permitted.
func (p TaskPolicy) AllowsCommand(command string) bool {
	if !p.AllowsTests() {
		return false
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}
	if len(p.Constraints.AllowedChecks) == 0 {
		return true
	}
	for _, allowed := range p.Constraints.AllowedChecks {
		if strings.EqualFold(strings.TrimSpace(allowed), command) {
			return true
		}
	}
	// Static argv prefix match: "only run go test" may allow "go test ./...",
	// but shell operators, substitutions, redirects, and extra commands do not
	// inherit that allowance.
	commandFields, malformed := shellparse.StaticFields(command)
	if malformed != "" || len(commandFields) == 0 {
		return false
	}
	for _, allowed := range p.Constraints.AllowedChecks {
		allowedFields, malformed := shellparse.StaticFields(strings.TrimSpace(allowed))
		if malformed == "" && len(allowedFields) > 0 && hasFieldPrefix(commandFields, allowedFields) {
			return true
		}
	}
	return false
}

func hasFieldPrefix(fields, prefix []string) bool {
	if len(prefix) > len(fields) {
		return false
	}
	for i := range prefix {
		if !strings.EqualFold(fields[i], prefix[i]) {
			return false
		}
	}
	return true
}

// AllowsExternal reports whether push/publish-style actions may run.
func (p TaskPolicy) AllowsExternal() bool {
	return !p.Constraints.ForbidExternal
}

func parseConstraints(instruction string) Constraints {
	var c Constraints
	lower := strings.ToLower(instruction)
	// Analysis-only / no modifications
	if matchesAny(lower, []string{
		"只分析", "只读", "不要修改", "别改", "不要改", "仅分析", "只看不改",
		"analyze only", "analysis only", "don't modify", "do not modify",
		"don't change", "do not change", "no changes", "read only", "read-only",
		"without modifying", "without changes", "don't edit", "do not edit",
	}) {
		c.ForbidMutation = true
		c.Notes = append(c.Notes, "user_forbid_mutation")
	}
	// Reproduce but don't fix
	if matchesAny(lower, []string{
		"复现但不修复", "只复现", "不要修复", "reproduce but don't fix",
		"reproduce only", "don't fix", "do not fix", "no fix",
	}) {
		c.ForbidMutation = true
		c.Notes = append(c.Notes, "user_reproduce_only")
	}
	// No tests
	if matchesAny(lower, []string{
		"不要测试", "别跑测试", "不用测试", "跳过测试", "不要跑测试",
		"don't run tests", "do not run tests", "no tests", "skip tests",
		"without tests", "don't test", "do not test",
	}) {
		c.ForbidTests = true
		c.Notes = append(c.Notes, "user_forbid_tests")
	}
	// Only run X
	if cmds := parseAllowedChecks(instruction); len(cmds) > 0 {
		c.AllowedChecks = cmds
		c.Notes = append(c.Notes, "user_allowed_checks")
	}
	// No push / no publish
	if matchesAny(lower, []string{
		"不要 push", "不要push", "别 push", "别push", "不要推送", "不要发布",
		"don't push", "do not push", "no push", "don't publish", "do not publish",
		"no publish", "don't deploy", "do not deploy",
	}) {
		c.ForbidExternal = true
		c.Notes = append(c.Notes, "user_forbid_external")
	}
	return c
}

func parseAllowedChecks(instruction string) []string {
	// Patterns: "只跑 X", "only run X", "just run X"
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)只跑\s+([^\n,，;；]+)`),
		regexp.MustCompile(`(?i)只运行\s+([^\n,，;；]+)`),
		regexp.MustCompile(`(?i)only\s+run\s+([^\n,;]+)`),
		regexp.MustCompile(`(?i)just\s+run\s+([^\n,;]+)`),
	}
	var out []string
	for _, re := range patterns {
		m := re.FindStringSubmatch(instruction)
		if len(m) < 2 {
			continue
		}
		cmd := strings.TrimSpace(m[1])
		cmd = strings.Trim(cmd, "\"'`。.")
		if cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func matchesAny(lower string, needles []string) bool {
	for _, n := range needles {
		if n == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// StripQuotedConstraints removes fenced code blocks and quoted spans so
// constraint phrases inside citations do not bind the host.
func StripQuotedConstraints(raw string) string {
	s := raw
	// Fenced code blocks ``` ... ```
	s = stripFences(s)
	// Inline code `...`
	s = stripInlineCode(s)
	// Double-quoted and Chinese quotation spans (non-greedy, single line-ish)
	s = stripQuoted(s, '"', '"')
	s = stripQuoted(s, '“', '”')
	s = stripQuoted(s, '「', '」')
	return strings.TrimSpace(s)
}

func stripFences(s string) string {
	var b strings.Builder
	inFence := false
	for line := range strings.SplitSeq(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func stripInlineCode(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '`' {
			in = !in
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripQuoted(s string, open, close rune) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if !in && r == open {
			in = true
			continue
		}
		if in && r == close {
			in = false
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ExecutionPolicyBlock renders the short provider-visible transient user block
// that freezes the role setting for this turn. Callers persist it in Message
// Content and keep the original user text in RawContent.
func ExecutionPolicyBlock(p TaskPolicy) string {
	var b strings.Builder
	b.WriteString(`<execution-policy preset="`)
	b.WriteString(p.Preset.String())
	b.WriteString(`" version="`)
	b.WriteString(strconv.Itoa(p.PolicyVersion))
	b.WriteString(`">`)
	b.WriteByte('\n')
	b.WriteString("verify=")
	b.WriteString(verifyName(p.Verification))
	if p.Constraints.ForbidMutation {
		b.WriteString("\nconstraint=no-mutation")
	}
	if p.Constraints.ForbidTests {
		b.WriteString("\nconstraint=no-tests")
	}
	if p.Constraints.ForbidExternal {
		b.WriteString("\nconstraint=no-external")
	}
	if len(p.Constraints.AllowedChecks) > 0 {
		b.WriteString("\nconstraint=only-checks:")
		b.WriteString(strings.Join(p.Constraints.AllowedChecks, ","))
	}
	if p.Constraints.PlanModeReadOnly {
		b.WriteString("\nconstraint=plan-mode-read-only")
	}
	b.WriteString("\n</execution-policy>")
	return b.String()
}

func verifyName(v Verification) string {
	switch v {
	case VerifyTargeted:
		return "targeted"
	case VerifyFull:
		return "full"
	default:
		return "none"
	}
}

// HasInstructionalContent reports whether s has non-space runes.
func HasInstructionalContent(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
