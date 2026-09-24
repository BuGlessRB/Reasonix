package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reasonix/internal/runtime/writeclaim"
	"reasonix/internal/state/checkpoint"
	"reasonix/internal/state/sessionstore"
	"reasonix/internal/state/sessiontemp"
	"slices"
	"strings"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/planmode"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/safety/evidence"
	"reasonix/internal/state/memory"
	"reasonix/internal/tools/jobs"
)

// RunSubAgentWithSession continues an existing sub-agent session with prompt and
// returns the latest final assistant answer. Fresh sub-agents pass a newly-created
// session; continued sub-agents pass a loaded transcript session.
//
// Each call installs an independent session-private temporary directory Manager
// so parent, sibling, and nested sub-agents never share temporary files.
// continue_from restores conversation history only — a new run still gets a
// fresh temporary directory.
func RunSubAgentWithSession(ctx context.Context, prov provider.Provider, reg *tool.Registry, sess *sessionstore.Session, prompt string, opts Options, sink event.Sink) (answer string, err error) {
	if sess == nil {
		return "", fmt.Errorf("sub-agent session is nil")
	}
	// Isolate temporary files for this run before any tool execution.
	ctx = tool.WithoutGoalTurnRecorder(ctx)
	if opts.MemoryQueue != nil {
		ctx = memory.WithQueue(ctx, opts.MemoryQueue)
	} else {
		ctx = memory.WithoutQueue(ctx)
	}
	if opts.Jobs == nil {
		ctx = jobs.WithoutManager(ctx)
	}
	ctx, releaseTemp := withSubagentSessionTemp(ctx)
	defer releaseTemp()
	if opts.SubagentDepth > 0 {
		ctx = WithSubagentDepth(ctx, opts.SubagentDepth)
	}
	// Callers that wrap the prompt themselves (runSubSession) set
	// ClassifierTaskText before wrapping; for everyone else the prompt is
	// still pristine here, so capture it before host framing is prepended.
	if strings.TrimSpace(opts.ClassifierTaskText) == "" {
		opts.ClassifierTaskText = prompt
	}
	planWorkflow := PlanModeFromContext(ctx)
	if opts.SubagentDepth > 0 && isFreshSubagentSession(sess) {
		prompt = subagentStartContext + "\n\n" + prompt
	}
	if planWorkflow && !strings.Contains(prompt, planmode.Marker) {
		prompt = planmode.Marker + "\n\n" + prompt
	}
	if kind := opts.RequireReviewReportKind; kind != "" {
		prompt = prompt + "\n\n" + reviewReportTaskContract(kind)
	}
	// Nested reasoning stays isolated; the parent consumes only final Content.
	// Require it so a reasoning-only stop cannot fall back to older tool text.
	opts.RequireVisibleFinal = true
	sub := newDelegatedAgent(ctx, prov, reg, sess, opts, sink)
	// One defer: this returns from a salvage, a review failure, a provider
	// error and a clean answer, and per-path calls would miss one.
	defer func() { observeSubagentHandoff(sink, sub, sess, opts, answer, err) }()
	sub.SetPlanMode(planWorkflow)
	if err := sub.Run(ctx, prompt); err != nil {
		// Still merge any partial child evidence so parent gates see real writes.
		mergeChildEvidence(ctx, sub)
		if answer, ok := salvageReadinessExhaustedAnswer(sub, sess, opts, err); ok {
			return composeSubagentAnswer(ctx, answer, sub, writeclaim.SubagentWriteClaim(ctx), opts.ClassifierTaskText), nil
		}
		return "", fmt.Errorf("sub-agent: %w", err)
	}
	// Review subagents hand back a typed report the parent's gate can verify.
	// A run that finished without one is nudged on the same session, evidence
	// preserved so review_report can cite the reads it already earned.
	if kind := opts.RequireReviewReportKind; kind != "" {
		nudges := 0
		for !sub.HasSuccessfulReviewReport(kind) && nudges < maxReviewReportNudges {
			nudges++
			sub.pending.preserveEvidence = true
			if err := sub.Run(ctx, reviewReportNudgePrompt(kind)); err != nil {
				mergeChildEvidence(ctx, sub)
				// A retry that fails still keeps local parent mutations; the
				// parent turns this into Partial/Unverified rather than rolling back.
				return "", fmt.Errorf("sub-agent: %w", err)
			}
		}
		if !sub.HasSuccessfulReviewReport(kind) {
			mergeChildEvidence(ctx, sub)
			// Nudging pays at every role setting: a verdict the host can read is
			// how a block reaches the gate. Failing over its absence does not —
			// no report is no block, the state a killed run leaves anyway.
			if !opts.DeliveryProfile {
				if answer := latestAssistantAnswer(sess); answer != "" {
					return composeSubagentAnswer(ctx, reviewWithoutVerdictNote+answer, sub, writeclaim.SubagentWriteClaim(ctx), opts.ClassifierTaskText), nil
				}
			}
			dumpRef := dumpFailedSubagentSession(opts.ArchiveDir, string(kind), sess)
			// Partial path: local changes are retained; the parent readiness
			// layer treats missing review as Partial/Unverified (not rollback).
			return "", &ReviewUnavailableError{
				Kind:   string(kind),
				Nudges: nudges,
				Dump:   dumpRef,
			}
		}
	}
	mergeChildEvidence(ctx, sub)
	if answer := latestAssistantAnswer(sess); answer != "" {
		return composeSubagentAnswer(ctx, answer, sub, writeclaim.SubagentWriteClaim(ctx), opts.ClassifierTaskText), nil
	}
	return "", fmt.Errorf("sub-agent finished without producing a final answer")
}

// readOnlyAgentConstruction is the single pairing every strictly read-only
// loop shares: the permanent ReadOnlyExecution flag plus the final registry
// filter. Batch children (RunReadOnlySubAgentWithSession) and legacy call sites
// that still use NewReadOnlyAgent build through it, so a missed call site
// cannot set only half the boundary. The interactive two-model planner uses
// NewPlannerAgent instead (PlannerMCPExecution).
func readOnlyAgentConstruction(reg *tool.Registry, opts Options) (*tool.Registry, Options) {
	opts.ReadOnlyExecution = true
	opts.PlannerMCPExecution = false
	return strictReadOnlyExecutionRegistry(reg), opts
}

// NewPlannerAgent constructs the interactive two-model planner: permanent
// ReadOnlyExecution still blocks bash, file writers, and ordinary non-MCP
// writers, while PlannerMCPExecution allows authorized, non-destructive MCP
// through the stable use_capability proxy without requiring readOnlyHint.
func NewPlannerAgent(prov provider.Provider, reg *tool.Registry, sess *sessionstore.Session, opts Options, sink event.Sink) *Agent {
	opts.EventSource = event.UsageSourcePlanner
	opts.ReadOnlyExecution = true
	opts.PlannerMCPExecution = true
	// The coordinator needs visible plan text to hand off to the executor;
	// reasoning shown in a frontend is not a substitute for that contract.
	opts.RequireVisibleFinal = true
	// Keep construction-time filter for ordinary tools; use_capability stays
	// because it is ReadOnly. Direct mcp__* tools are already excluded by
	// PlannerToolRegistry. Dynamic MCP targets are re-checked after resolve.
	reg = plannerExecutionRegistry(reg)
	return New(prov, reg, sess, opts, sink)
}

// plannerExecutionRegistry is the construction-time filter for NewPlannerAgent.
// It removes ordinary writers and destructive direct MCP tools while keeping
// use_capability and built-in research tools. Host-starting deferred MCP
// targets are allowed at execution time under PlannerMCPExecution.
func plannerExecutionRegistry(reg *tool.Registry) *tool.Registry {
	filtered := tool.NewRegistry()
	if reg == nil {
		return filtered
	}
	for _, name := range reg.Names() {
		target, ok := reg.Get(name)
		if !ok {
			continue
		}
		if name == "use_capability" {
			filtered.Add(target)
			continue
		}
		if strings.HasPrefix(name, tool.MCPNamePrefix) {
			// Defense in depth: planner never exposes direct MCP schemas.
			continue
		}
		if !target.ReadOnly() || mcpDestructiveHint(target) {
			continue
		}
		if h, ok := target.(tool.ReadOnlyExecutionHostMutation); ok && h.ReadOnlyExecutionHostMutation() {
			// Ordinary host mutations stay out; MCP startup is only via proxy.
			continue
		}
		filtered.Add(target)
	}
	return filtered
}

// RunReadOnlySubAgentWithSession is the construction boundary for every
// strictly read-only child loop. Registry filtering limits the visible surface;
// this permanent execution flag also re-checks targets resolved dynamically by
// proxy tools such as use_capability. It never enables PlannerMCPExecution.
func RunReadOnlySubAgentWithSession(ctx context.Context, prov provider.Provider, reg *tool.Registry, sess *sessionstore.Session, prompt string, opts Options, sink event.Sink) (string, error) {
	reg, opts = readOnlyAgentConstruction(reg, opts)
	return RunSubAgentWithSession(ctx, prov, reg, sess, prompt, opts, sink)
}

// strictReadOnlyExecutionRegistry is the final construction-time filter shared
// by every strict child. Callers still apply role-specific filtering (review,
// planner, profile allowlists), while this layer guarantees that a missed call
// site cannot expose writers, destructive MCP tools, readers from unauthorized
// servers, or an unauthorized host-starting target to the model.
func strictReadOnlyExecutionRegistry(reg *tool.Registry) *tool.Registry {
	filtered := tool.NewRegistry()
	if reg == nil {
		return filtered
	}
	for _, name := range reg.Names() {
		target, ok := reg.Get(name)
		if !ok || !target.ReadOnly() || mcpDestructiveHint(target) {
			continue
		}
		if isInstalledMCPTool(target) && !mcpServerAuthorized(target) {
			continue
		}
		if mutation, ok := target.(tool.ReadOnlyExecutionHostMutation); ok && mutation.ReadOnlyExecutionHostMutation() && !readOnlyExecutionAllowsMCPStartup(target) {
			continue
		}
		filtered.Add(target)
	}
	return filtered
}

// latestAssistantAnswer walks the session backwards for the last assistant
// message with content — that's the sub-agent's final answer. Intermediate
// assistant messages with tool_calls but no text don't count.
func latestAssistantAnswer(sess *sessionstore.Session) string {
	if sess == nil {
		return ""
	}
	for _, v := range slices.Backward(sess.Messages) {
		m := v
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content
		}
	}
	return ""
}

// dumpFailedSubagentSession best-effort persists a failed report-required
// subagent transcript for post-hoc diagnosis (read-only skill subagents are
// otherwise ephemeral, so a protocol failure leaves no trace). Returns a
// human-readable suffix naming the dump, or "" when disabled/failed.
func dumpFailedSubagentSession(archiveDir, kind string, sess *sessionstore.Session) string {
	if strings.TrimSpace(archiveDir) == "" || sess == nil {
		return ""
	}
	dir := filepath.Join(archiveDir, "subagent-report-failures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", kind, time.Now().UnixNano()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return ""
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range sess.Messages {
		if err := enc.Encode(m); err != nil {
			return ""
		}
	}
	return "; transcript dumped to " + path
}

// mergeChildEvidence folds a sub-agent's real receipts into the parent ledger
// carried on ctx. Meta tools themselves are never mutations.
func mergeChildEvidence(ctx context.Context, sub *Agent) {
	if sub == nil {
		return
	}
	parent, ok := evidence.FromContext(ctx)
	if !ok || parent == nil {
		return
	}
	parent.MergeChild(sub.EvidenceSummary())
}

func isFreshSubagentSession(sess *sessionstore.Session) bool {
	if sess == nil {
		return false
	}
	snap := sess.Snapshot()
	return len(snap) == 1 && snap[0].Role == provider.RoleSystem
}

// EvidenceSummary exports this agent's turn-scoped receipts for parent merge.
func (a *Agent) EvidenceSummary() evidence.ChildEvidenceSummary {
	if a == nil || a.task.ledger == nil {
		return evidence.ChildEvidenceSummary{}
	}
	return a.task.ledger.Summary()
}

// composeSubagentAnswer assembles everything the parent is shown for one child
// run: the host-adjudicated completion claim when the child submitted one, the
// child's own prose, then the host's receipts.
func composeSubagentAnswer(ctx context.Context, answer string, sub *Agent, claims writeclaim.WritePathSet, delegationText string) string {
	summary := sub.EvidenceSummary()
	report, reasons, hasReport := sub.CompletionReport()
	if hasReport {
		answer = strings.TrimSpace(formatCompletionReport(report, reasons) + "\n\n" + answer)
	}
	recordDelegationAudit(ctx, summary, claims, report, reasons, hasReport, delegationText)
	return appendHostReceipts(answer, summary, claims)
}

// maxReviewReportNudges bounds the in-session completion nudges sent to a
// review subagent that finished without submitting review_report. Each nudge is
// one cheap continuation request on the same (cached) subagent session — far
// cheaper than discarding the run and re-reviewing from scratch.
// maxReviewReportNudges is the single in-session retry after a review run that
// produced no typed report. One is the useful number: the nudge asks only for
// the submission, and a second refusal means the tool is unreachable rather
// than forgotten, which no further asking fixes.
const maxReviewReportNudges = 1

// newDelegatedAgent builds the child a delegated run executes as, wired from
// the call context rather than from what a caller remembered to pass. Only a
// delegated run gets an asker: its parent is blocked and a person is waiting,
// while fleet items, parallel tasks and background jobs stay silent.
func newDelegatedAgent(ctx context.Context, prov provider.Provider, reg *tool.Registry,
	sess *sessionstore.Session, opts Options, sink event.Sink,
) *Agent {
	sub := New(prov, reg, sess, opts, sink)
	if _, _, asker, ok := CallContext(ctx); ok && asker != nil {
		sub.SetAsker(asker)
	}
	return sub
}

// observeSubagentHandoff records one child run's closing protocol. Called from
// a single defer so every exit path is counted the same: a salvage, a review
// failure and a clean answer are all runs that either submitted a report or
// did not.
func observeSubagentHandoff(sink event.Sink, sub *Agent, sess *sessionstore.Session, opts Options, answer string, runErr error) {
	if sub == nil || sess == nil {
		return
	}
	audit := event.SubagentHandoffAudit{
		Entrance: opts.HandoffEntrance,
		Depth:    opts.SubagentDepth,
		ReadOnly: opts.ReadOnlyExecution,
		Expected: opts.ExpectCompletionReport,
		Exit:     handoffExit(answer, runErr),
	}
	countReportCalls(&audit, sess.Snapshot())
	// Claimed and adjudicated are both kept: a child that submits reports
	// readily and has them lowered every time is a different problem from one
	// that does not submit, and a single status cannot say which.
	if sub.task.ledger != nil {
		verdicts := sub.task.ledger.ClosureVerdicts()
		audit.Closed, audit.NeedsWork = verdicts.Closed, verdicts.NeedsWork
		if claimed, ok := sub.task.ledger.LatestCompletionReport(); ok {
			adjudicated, reasons := sub.task.ledger.AdjudicateCompletion(claimed)
			audit.ClaimedStatus = string(claimed.Status)
			audit.AdjudicatedStatus = string(adjudicated.Status)
			audit.LoweredClaims = len(reasons)
			audit.Criteria = len(adjudicated.Criteria)
			audit.Unresolved = len(adjudicated.Unresolved)
			for _, c := range adjudicated.Criteria {
				audit.Evidence += len(c.Evidence)
			}
		}
	}
	event.RecordSubagentHandoff(sink, audit)
}

// reviewReportNudgePrompt asks an already-finished review subagent to submit
// the missing typed report without redoing the review.
func reviewReportNudgePrompt(kind evidence.ReviewKind) string {
	return fmt.Sprintf("You finished the review without calling the review_report tool, so the host cannot accept the run yet. Do not redo the review. Call review_report now with kind=%q, your verdict (pass | warn | block), reviewed_paths listing only the files you actually read in this conversation, and the findings you already reported. Then restate your final verdict in one sentence.", string(kind))
}

// reviewReportTaskContract is appended to the task prompt of a review subagent
// whose run must end with a typed report. The skill body describes how to
// review; this states the non-negotiable submission protocol.
func reviewReportTaskContract(kind evidence.ReviewKind) string {
	return fmt.Sprintf(`<review-report-contract event="SubagentReviewReport">
Before your final answer you MUST call the review_report tool exactly once with kind=%q, your verdict (pass | warn | block), reviewed_paths listing only files you actually read this run, and your findings. The host discards a review run that ends without a successful review_report call — your prose summary alone does not count.
</review-report-contract>`, string(kind))
}

// reviewWithoutVerdictNote marks a review the host has no typed verdict for, so
// the prose is not read as one the gate weighed and let through.
const reviewWithoutVerdictNote = "[no recorded verdict] The reviewer did not submit a review_report, so the host holds no verdict for this run: nothing below was checked against its receipts, and a blocking finding here will not stop delivery on its own.\n\n"

const subagentStartContext = `<subagent-context event="SubagentStart">
Before acting, check the available skills and tools. If a relevant skill is available, invoke it before continuing. Delegate to another sub-agent only when the task genuinely benefits from isolated context and the delegation tool is available.
</subagent-context>`

// withSubagentSessionTemp installs a fresh session-private temporary directory
// Manager for one sub-agent run. The returned release must be deferred by the
// caller so the directory is retired when the run ends (including background
// sub-agent completion).
func withSubagentSessionTemp(ctx context.Context) (context.Context, func()) {
	m := sessiontemp.New()
	m.Retain()
	return sessiontemp.WithManager(ctx, m), m.Release
}

// mutationObserverSetter is a tool that spawns sub-agents and hands them the
// parent's mutation observer.
type mutationObserverSetter interface {
	SetMutationObserver(*checkpoint.MutationObserver)
}
