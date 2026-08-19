package boot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/skill"
	"reasonix/internal/tool"
)

// skillSubagents runs a skill inside its own sub-agent loop: an isolated turn
// with the skill body as system prompt, a tool set scoped to the skill's
// allowed tools minus recursive meta-tools, and an optional per-skill model.
// capRuntime is filled in after the capability runtime exists; nothing calls a
// runner before then, because the tools holding them only fire on a model request.
type skillSubagents struct {
	root       string
	cfg        *config.Config
	registry   *tool.Registry
	scheduler  *agent.SubagentScheduler
	store      *agent.SubagentStore
	provider   provider.Provider
	entry      *config.ProviderEntry
	capRuntime *agent.MCPCapabilityRuntime
	maxDepth   int
	maxSteps   int

	resolveProvider func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error)
	identity        func(modelRef, effort string) (string, string)
	runOptions      func(ctx context.Context, steps int, price *provider.Pricing, ctxWin, childDepth int) agent.Options
}

// halfSteps gives a child half the parent's step budget, never below five:
// enough to finish a small job, not enough to spend the parent's turn.
func (r *skillSubagents) halfSteps() int {
	steps := r.maxSteps
	if steps > 0 {
		if steps /= 2; steps < 5 {
			steps = 5
		}
	}
	return steps
}

// resolveModel picks the child's provider: the parent's unless the skill or
// config names one of its own.
func (r *skillSubagents) resolveModel(sk skill.Skill) (provider.Provider, *provider.Pricing, int, string, string, error) {
	prov, price, ctxWin := r.provider, r.entry.Price, r.entry.ContextWindow
	modelRef := subagentModelRef(r.cfg, sk)
	effortRef := subagentEffortRef(r.cfg, sk)
	if modelRef == "" && effortRef == "" {
		return prov, price, ctxWin, modelRef, effortRef, nil
	}
	p, pr, cw, err := r.resolveProvider(modelRef, effortRef)
	if err != nil {
		return nil, nil, 0, "", "", err
	}
	return p, pr, cw, modelRef, effortRef, nil
}

func (r *skillSubagents) runReadOnly(sctx context.Context, sk skill.Skill, task string, runOpts skill.SubagentRunOptions) (string, error) {
	if strings.TrimSpace(runOpts.ContinueFrom) != "" || strings.TrimSpace(runOpts.ForkFrom) != "" {
		return "", fmt.Errorf("read_only_skill does not support continue_from/fork_from")
	}
	releaseSlot, err := r.scheduler.Acquire(sctx, agent.AcquireRequest{
		Writer: false,
		Nested: agent.SubagentDepth(sctx) > 0,
		Label:  sk.Name,
	})
	if err != nil {
		return "", err
	}
	defer releaseSlot()
	sk = skill.WithCodeGraphTools(sk, skill.CodeGraphReadTools(r.registry))
	prov, price, ctxWin, modelRef, effortRef, err := r.resolveModel(sk)
	if err != nil {
		return "", fmt.Errorf("read-only subagent skill %q profile: %w", sk.Name, err)
	}
	childDepth := agent.SubagentDepth(sctx) + 1
	if childDepth > r.maxDepth {
		return "", fmt.Errorf("subagent delegation depth limit reached (max_subagent_depth=%d)", r.maxDepth)
	}
	subReg := agent.ReadOnlySubagentToolRegistryForDepthWithRuntime(r.registry, sk.AllowedTools, childDepth, r.maxDepth, r.capRuntime)
	if subReg.Len() == 0 {
		return "", fmt.Errorf("read_only_skill: skill %q has no read-only tools available", sk.Name)
	}
	switch sk.Name {
	case "review", "security-review", "security_review":
		agent.AttachReviewReportTool(subReg)
	}
	// Custom and named built-in profiles fully control their system prompt
	// (no implicit concise/DefaultReadOnlyTaskSystemPrompt overlay).
	sysPrompt := strings.TrimSpace(sk.Body)
	if sysPrompt == "" {
		sysPrompt = agent.DefaultReadOnlyTaskSystemPrompt
	}
	runOptions := r.runOptions(sctx, r.halfSteps(), price, ctxWin, childDepth)
	usageModelRef, _ := r.identity(modelRef, effortRef)
	runOptions.ModelRef = usageModelRef
	// Delivery risk gates consume typed reports; outside Delivery a casual
	// /review run may finish with prose only.
	if runOptions.DeliveryProfile {
		runOptions.RequireReviewReportKind = agent.ReviewReportKindForSkill(sk.Name)
	}
	// Provider serializers decide whether these images are wire-visible from
	// the child model's own vision capability. Text-only children retain the
	// attachment metadata locally but never receive image parts on the wire.
	childCtx := agent.WithUserImages(sctx, agent.SubagentImageCandidates(sctx))
	return agent.RunReadOnlySubAgentWithSession(childCtx, prov, subReg, agent.NewSession(sysPrompt), task,
		runOptions, agent.NestedSink(sctx, event.Discard))
}

func (r *skillSubagents) run(sctx context.Context, sk skill.Skill, task string, runOpts skill.SubagentRunOptions) (string, error) {
	// Writer skills without write_paths claim the whole workspace so they
	// cannot race fleet/task writers that declared disjoint paths.
	acq := agent.AcquireRequest{
		Writer: !sk.ReadOnly,
		Nested: agent.SubagentDepth(sctx) > 0,
		Label:  sk.Name,
	}
	if !sk.ReadOnly {
		whole, werr := agent.WholeWorkspaceWriteClaim(r.root)
		if werr != nil {
			return "", fmt.Errorf("subagent skill %q write claim: %w", sk.Name, werr)
		}
		acq.WritePaths = whole
	}
	releaseSlot, err := r.scheduler.Acquire(sctx, acq)
	if err != nil {
		return "", err
	}
	defer releaseSlot()
	sk = skill.WithCodeGraphTools(sk, skill.CodeGraphReadTools(r.registry))
	prov, price, ctxWin, modelRef, effortRef, err := r.resolveModel(sk)
	if err != nil {
		return "", fmt.Errorf("subagent skill %q profile: %w", sk.Name, err)
	}
	childDepth := agent.SubagentDepth(sctx) + 1
	if childDepth > r.maxDepth {
		return "", fmt.Errorf("subagent delegation depth limit reached (max_subagent_depth=%d)", r.maxDepth)
	}
	// A read-only skill (builtin review/security-review, or frontmatter
	// `read-only: true`) gets its promise enforced at the tool boundary:
	// writer tools are stripped and bash runs under the read-only command
	// policy. Transcripts recorded against the writer-capable registry stop
	// matching on continue_from (schema-hash check reports the mismatch).
	var subReg *tool.Registry
	if sk.ReadOnly {
		subReg = agent.ReadOnlySubagentToolRegistryForDepthWithRuntime(r.registry, sk.AllowedTools, childDepth, r.maxDepth, r.capRuntime)
	} else {
		subReg = agent.SubagentToolRegistryForDepthWithRuntime(r.registry, sk.AllowedTools, childDepth, r.maxDepth, r.capRuntime)
	}
	// Delivery risk gates require structured review_report from review
	// subagents only — never expose it on the parent tool surface.
	switch sk.Name {
	case "review", "security-review", "security_review":
		agent.AttachReviewReportTool(subReg)
	}
	run, err := r.prepareRun(sctx, sk, subReg, runOpts, modelRef, effortRef)
	if err != nil {
		return "", err
	}
	defer run.Release()
	runOptions := r.runOptions(sctx, r.halfSteps(), price, ctxWin, childDepth)
	usageModelRef, _ := r.identity(modelRef, effortRef)
	runOptions.ModelRef = usageModelRef
	// Delivery risk gates consume typed reports; outside Delivery a casual
	// /review run may finish with prose only.
	if runOptions.DeliveryProfile {
		runOptions.RequireReviewReportKind = agent.ReviewReportKindForSkill(sk.Name)
	}
	var answer string
	// See runReadOnly: the child provider, not the parent model, owns the
	// final vision decision.
	childCtx := agent.WithUserImages(sctx, agent.SubagentImageCandidates(sctx))
	if sk.ReadOnly {
		answer, err = agent.RunReadOnlySubAgentWithSession(childCtx, prov, subReg, run.Session, task,
			runOptions, agent.NestedSink(sctx, event.Discard))
	} else {
		answer, err = agent.RunSubAgentWithSession(childCtx, prov, subReg, run.Session, task,
			runOptions, agent.NestedSink(sctx, event.Discard))
	}
	if err != nil {
		return "", errors.Join(err, r.store.SaveFailed(run))
	}
	if err := r.store.SaveCompleted(run); err != nil {
		return "", errors.Join(err, r.store.SaveFailed(run))
	}
	return agent.FormatSubagentRunResult(answer, run, false), nil
}

// prepareRun opens the child's transcript. Headless runs have no persistent
// session to own one, so the skill runs ephemerally instead of failing —
// continuation still errors there, because it needs a persisted owner.
func (r *skillSubagents) prepareRun(sctx context.Context, sk skill.Skill, subReg *tool.Registry, runOpts skill.SubagentRunOptions, modelRef, effortRef string) (*agent.SubagentRun, error) {
	continueFrom := strings.TrimSpace(runOpts.ContinueFrom)
	legacyForkFrom := strings.TrimSpace(runOpts.ForkFrom)
	if continueFrom != "" && legacyForkFrom != "" {
		return nil, fmt.Errorf("continue_from and fork_from are mutually exclusive; pass only continue_from")
	}
	parentID, _, _, _ := agent.CallContext(sctx)
	if runOpts.HostInitiated {
		parentID = ""
	}
	parentSession := agent.ParentSession(sctx)
	if r.store == nil || parentSession == "" {
		if continueFrom != "" || legacyForkFrom != "" {
			return nil, fmt.Errorf("subagent continuation requires a persisted session; none is active in this run")
		}
		return agent.EphemeralSubagentRun(sk.Body), nil
	}
	identityModel, identityEffort := r.identity(modelRef, effortRef)
	spec := agent.SubagentSpec{
		Kind:             "skill",
		Name:             sk.Name,
		WorkspaceRoot:    r.root,
		ParentSession:    parentSession,
		ParentToolCallID: parentID,
		SystemPrompt:     sk.Body,
		Registry:         subReg,
		Model:            identityModel,
		Effort:           identityEffort,
	}
	switch {
	case continueFrom != "":
		return r.store.PrepareContinue(continueFrom, spec)
	case legacyForkFrom != "":
		return r.store.PrepareLegacyForkFrom(legacyForkFrom, spec)
	default:
		return r.store.PrepareFresh(spec)
	}
}
