package control

// The approval bridge: what answers the agent's permission gate, and the text
// each kind of ask shows a human. It is declared sensitive in REASONIX.md
// because these decide whether a tool call runs.

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/tool"
	"reasonix/internal/safety/permission"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/state/memory"
)

// denyPermissionApprover answers for a session nobody is watching: a headless
// run has no prompt to show, so a call that needs approval can only be refused.
type denyPermissionApprover struct{}

func (denyPermissionApprover) Approve(context.Context, string, string, json.RawMessage) (bool, bool, error) {
	return false, false, nil
}

// ApproveWithReason says which refusal this is: without a reason the gate reports
// "the user declined this tool call", untrue when there was no user, and the
// model goes to ask someone who was never there. A call only a person may
// answer leads with why, so the model learns which of its steps needed one.
func (denyPermissionApprover) ApproveWithReason(_ context.Context, tool, subject string, _ json.RawMessage) (bool, bool, string, error) {
	reason := "this session has no interactive approver, so any call that needs approval is refused — nobody declined it, and neither retrying nor rewriting it can change that. If this work is meant to run unattended, it needs a permission mode that does not ask (or an explicit allow rule for this tool). Otherwise do the part that needs no approval and call conclude_blocked naming what was refused."
	if why := explicitApprovalReason(tool, subject); why != "" {
		reason = why + " " + reason
	}
	return false, false, reason, nil
}

// rulesWithoutFreshHumanApproval drops any session-allow rule that targets a
// tool requiring fresh human approval, so an explicit allowlist cannot bypass
// the always-prompt contract for those tools.
func rulesWithoutFreshHumanApproval(rules []permission.Rule) []permission.Rule {
	if len(rules) == 0 {
		return rules
	}
	filtered := make([]permission.Rule, 0, len(rules))
	for _, r := range rules {
		if RequiresFreshHumanApprovalTool(r.Tool) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// approval bridge (agent gate → events)

// gateApprover adapts the Controller to permission.Approver. It is distinct
// from the public Approve command (different signature, different direction).
type gateApprover struct{ c *Controller }

// The classes of call no posture answers for. The code is the identity a
// window renders in the reader's language; the sentence is what the model is
// told, and it stays in the language the model is addressed in.
const (
	dynamicBashApproval      = "dynamic_bash"
	browserCredentialApprova = "browser_credential"
	browserScriptApproval    = "browser_script"
	computerUseApproval      = "computer_use"
	computerPointerApproval  = "computer_pointer"
	computerFrontApproval    = "computer_front"
)

// computerActTakesFront is whether operating an application can take the
// foreground: Windows delivers keys only to the window in front.
var computerActTakesFront = runtime.GOOS == "windows"

var explicitApprovalTexts = map[string]string{
	dynamicBashApproval:      "This command uses nested or indirect shell execution. Auto and broad allow rules cannot verify the inner command; approve this exact command or use YOLO.",
	browserCredentialApprova: "This browser step types a password, a one-time code or card details into the site. Auto, the site's grant and broad allow rules do not answer it; approve it or use YOLO.",
	browserScriptApproval:    "This browser step runs arbitrary JavaScript with the page's authority. Auto, the site's grant and broad allow rules do not answer it; approve it or use YOLO.",
	computerPointerApproval:  "This takes the pointer the person is holding — it moves their cursor, clicks with it, and brings the application forward — rather than asking an element to act. Auto, the application's own grant and broad allow rules do not answer it; approve it or use YOLO.",
	computerUseApproval:      "This reads or operates another application on the computer, with whatever access that application has. Auto and broad allow rules do not answer it; approve it for this application or use YOLO.",
	computerFrontApproval:    "This operates another application on the computer, with whatever access that application has. Windows sends keys only to the window in front, so typing, key presses and pastes bring that application forward first. Auto and broad allow rules do not answer it; approve it for this application or use YOLO.",
}

// ExplicitApprovalCode names why a call needs a person rather than auto or a
// broad rule, or "" when it does not. It is derived from the call, so whoever
// renders the prompt asks rather than being told.
func ExplicitApprovalCode(tool, subject string) string {
	switch {
	case strings.EqualFold(tool, "bash") && permission.BashSubjectRequiresExplicitApproval(subject):
		return dynamicBashApproval
	case permission.IsBrowserTool(tool) && strings.HasPrefix(subject, permission.BrowserScriptPrefix):
		return browserScriptApproval
	case permission.IsBrowserTool(tool) && permission.BrowserSubjectRequiresExplicitApproval(subject):
		return browserCredentialApprova
	case permission.IsComputerTool(tool) && permission.ComputerSubjectTakesPointer(subject):
		return computerPointerApproval
	case permission.IsComputerTool(tool) && tool == "computer_act" && computerActTakesFront:
		return computerFrontApproval
	case permission.IsComputerTool(tool):
		return computerUseApproval
	}
	return ""
}

// explicitApprovalReason is that same answer as the sentence the model reads.
func explicitApprovalReason(tool, subject string) string {
	return explicitApprovalTexts[ExplicitApprovalCode(tool, subject)]
}

func (g gateApprover) Approve(ctx context.Context, tool, subject string, args json.RawMessage) (bool, bool, error) {
	allow, remember, _, err := g.ApproveWithReason(ctx, tool, subject, args)
	return allow, remember, err
}

func (g gateApprover) ApproveWithReason(ctx context.Context, tool, subject string, args json.RawMessage) (bool, bool, string, error) {
	return g.approveWithPolicyReason(ctx, tool, subject, args, "")
}

func (g gateApprover) ApproveWithPolicyReason(ctx context.Context, tool, subject string, args json.RawMessage, policyReason string) (bool, bool, string, error) {
	return g.approveWithPolicyReason(ctx, tool, subject, args, policyReason)
}

func combineApprovalReasons(reasons ...string) string {
	var kept []string
	for _, reason := range reasons {
		if reason = strings.TrimSpace(reason); reason != "" {
			kept = append(kept, reason)
		}
	}
	return strings.Join(kept, "\n")
}

func (g gateApprover) approveWithPolicyReason(ctx context.Context, tool, subject string, args json.RawMessage, policyReason string) (bool, bool, string, error) {
	if tool == memoryRememberTool && g.c.allowLowRiskRemember(args) {
		return true, false, "", nil
	}
	subject = approvalDisplaySubject(tool, subject, args)
	humanReason := explicitApprovalReason(tool, subject)
	requireHuman := humanReason != ""
	// Check pre-approval first, before any prompt or Guardian review. Dynamic
	// Bash accepts only YOLO or an exact session grant here; ordinary calls also
	// accept the just-approved-plan window. Deny rules already bit at the policy
	// level before this point.
	if requireHuman && g.c.approval.preApprovedForRequiredHuman(tool, subject) {
		return true, false, "", nil
	}
	if !requireHuman && g.c.approval.preApproved(tool, subject, args) {
		return true, false, "", nil
	}
	if g.c.guardianSess != nil && !requireHuman {
		allow, reason, reviewErr := g.c.guardianSess.Review(ctx, tool, args, g.c.executor.Session())
		if reviewErr != nil {
			return false, false, "", reviewErr
		}
		if allow && !requiresFreshApprovalTool(tool) {
			return true, false, "", nil
		}
		reason = combineApprovalReasons(policyReason, reason)
		humanAllow, remember, err := g.c.requestApproval(ctx, approvalRequest{tool: tool, subject: subject, args: args, reason: reason})
		if err != nil {
			return false, false, reason, err
		}
		if !humanAllow {
			return false, false, reason, nil
		}
		return true, remember, "", nil
	}
	if requireHuman {
		reason := combineApprovalReasons(policyReason, humanReason)
		allow, remember, err := g.c.requestApproval(ctx, approvalRequest{tool: tool, subject: subject, args: args, reason: reason, requireHuman: true})
		return allow, remember, "", err
	}
	allow, remember, err := g.c.requestApproval(ctx, approvalRequest{tool: tool, subject: subject, args: args, reason: policyReason})
	return allow, remember, "", err
}

type sandboxEscapeApprover struct{ c *Controller }

func (s sandboxEscapeApprover) ApproveSandboxEscape(ctx context.Context, req sandbox.EscapeRequest) (bool, string, error) {
	subject := sandboxEscapeApprovalSubject(req.Command)
	reason := sandboxEscapeApprovalReason(req.Reason)
	reply, err := s.c.requestApprovalDecision(ctx, approvalRequest{tool: SandboxEscapeApprovalTool, subject: subject, args: req.Args, reason: reason, fresh: true})
	if err != nil {
		return false, "approval aborted", err
	}
	if !reply.allow {
		return false, i18n.M.SandboxEscapeDeclined, nil
	}
	if reply.session {
		s.c.approval.grantSession(SandboxEscapeApprovalTool, subject)
	}
	return true, "", nil
}

func (s sandboxEscapeApprover) SandboxEscapeSessionAllowed(_ context.Context, req sandbox.EscapeRequest) bool {
	return s.c.approval.preApprovedForDecision(SandboxEscapeApprovalTool, sandboxEscapeApprovalSubject(req.Command), nil, true)
}

// ApproveEgress asks whether bash may reach a host the allow list does not
// name. The user configured that list, so no approval mode answers for them;
// only a person, once or for the rest of the session, widens it.
func (s sandboxEscapeApprover) ApproveEgress(ctx context.Context, host string) (bool, error) {
	reply, err := s.c.requestApprovalDecision(ctx, approvalRequest{tool: NetworkEgressApprovalTool, subject: host,
		reason: fmt.Sprintf(i18n.M.EgressApprovalReasonFmt, host), fresh: true})
	if err != nil || !reply.allow {
		return false, err
	}
	if reply.session {
		s.c.approval.grantSession(NetworkEgressApprovalTool, host)
	}
	return true, nil
}

func sandboxEscapeApprovalSubject(command string) string {
	subject := strings.TrimSpace(command)
	if subject == "" {
		return i18n.M.SandboxEscapeSubjectFallback
	}
	return i18n.M.SandboxEscapeSubjectPrefix + subject
}

func sandboxEscapeApprovalReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return i18n.M.SandboxEscapeRuntimeReason
	}
	return reason
}

// managedConfigWriteApprover routes a file tool's Reasonix-managed config write
// through the fresh-human approval prompt (see ManagedConfigWriteApprovalTool).
// A session grant is tool-wide (mirroring sandbox_escape): one "allow for this
// session" covers the rest of the repair flow across the handful of managed
// config files without re-prompting on every incremental edit.
type managedConfigWriteApprover struct{ c *Controller }

func (m managedConfigWriteApprover) ApproveManagedConfigWrite(ctx context.Context, req tool.ConfigWriteRequest) (bool, string, error) {
	subject := managedConfigWriteApprovalSubject(req.Path)
	args, _ := json.Marshal(map[string]string{"path": req.Path})
	reply, err := m.c.requestApprovalDecision(ctx, approvalRequest{tool: ManagedConfigWriteApprovalTool, subject: subject, args: args, reason: i18n.M.ConfigWriteReason, fresh: true})
	if err != nil {
		return false, "approval aborted", err
	}
	if !reply.allow {
		return false, i18n.M.ConfigWriteDeclined, nil
	}
	if reply.session {
		m.c.approval.grantSession(ManagedConfigWriteApprovalTool, subject)
	}
	return true, "", nil
}

func (m managedConfigWriteApprover) ManagedConfigWriteSessionAllowed(_ context.Context, req tool.ConfigWriteRequest) bool {
	return m.c.approval.preApprovedForDecision(ManagedConfigWriteApprovalTool, managedConfigWriteApprovalSubject(req.Path), nil, true)
}

func managedConfigWriteApprovalSubject(path string) string {
	return i18n.M.ConfigWriteSubjectPrefix + strings.TrimSpace(path)
}

func approvalDisplaySubject(tool, subject string, args json.RawMessage) string {
	switch tool {
	case memoryRememberTool:
		return rememberApprovalSubject(subject, args)
	case memoryForgetTool:
		return forgetApprovalSubject(subject, args)
	case "move_file":
		return moveApprovalSubject(subject, args)
	default:
		return subject
	}
}

func moveApprovalSubject(fallback string, args json.RawMessage) string {
	if len(args) == 0 {
		return fallback
	}
	var in struct {
		SourcePath      string `json:"source_path"`
		DestinationPath string `json:"destination_path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fallback
	}
	if in.SourcePath == "" || in.DestinationPath == "" {
		return fallback
	}
	return in.SourcePath + " -> " + in.DestinationPath
}

func rememberApprovalSubject(fallback string, args json.RawMessage) string {
	if len(args) == 0 {
		return fallback
	}
	var in struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fallback
	}
	name := approvalCompactText(firstNonEmpty(in.Name, in.Title))
	desc := approvalTruncate(approvalCompactText(in.Description), 180)
	body := approvalTruncate(approvalCompactText(in.Body), 240)
	typ := string(memory.NormalizeType(in.Type))

	var b strings.Builder
	b.WriteString(i18n.M.MemoryApprovalSaveUpdate)
	baseLen := b.Len()
	if name != "" {
		fmt.Fprintf(&b, " %q", name)
	}
	if typ != "" {
		fmt.Fprintf(&b, " [%s]", typ)
	}
	if desc != "" {
		b.WriteString(": ")
		b.WriteString(desc)
	}
	if body != "" {
		if desc == "" {
			b.WriteString(": ")
		} else {
			b.WriteString(" | ")
		}
		b.WriteString(i18n.M.MemoryApprovalBodyLabel)
		b.WriteString(": ")
		b.WriteString(body)
	}
	if b.Len() == baseLen && fallback != "" {
		return fallback
	}
	return b.String()
}

func forgetApprovalSubject(fallback string, args json.RawMessage) string {
	if len(args) == 0 {
		return fallback
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fallback
	}
	name := approvalCompactText(in.Name)
	if name == "" {
		return fallback
	}
	return fmt.Sprintf(i18n.M.MemoryApprovalArchiveFmt, name)
}

func approvalCompactText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func approvalTruncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// approvalRequest is one ask: what is being approved, why, and which postures
// may answer it instead of a human. The zero value is an ordinary tool
// permission with no stated reason.
type approvalRequest struct {
	tool    string
	subject string
	args    json.RawMessage
	reason  string
	// fresh marks a user trust/business decision rather than an ordinary tool
	// permission. It may reuse an explicit session grant, but YOLO/auto approval
	// must not answer or drain the prompt.
	fresh bool
	// requireHuman marks an ordinary tool approval that Auto, an approved-plan
	// window, Guardian, or an allowing hook must not answer. Unlike fresh it
	// retains the ordinary four-choice UI and YOLO remains an explicit bypass.
	requireHuman bool
}
