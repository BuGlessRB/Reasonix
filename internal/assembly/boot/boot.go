// Package boot assembles a ready-to-drive control.Controller from configuration:
// it loads config, resolves the model(s), builds the tool registry (built-ins +
// plugins), wires the permission gate, and constructs the executor — optionally
// wrapping it in a two-model Coordinator. It is the one place that turns "what the
// user configured" into "a Controller a frontend can drive", so every frontend —
// the terminal TUI, the HTTP/SSE server, the desktop webview — shares the exact
// same assembly instead of each re-deriving it. Frontends pass only a sink and a
// couple of run knobs; everything else comes from config.
package boot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"reasonix/internal/base/netclient"
	"reasonix/internal/base/secrets"
	"reasonix/internal/contract/ablation"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/extension"
	"reasonix/internal/ext/extension/protocol"
	"reasonix/internal/ext/mcplaunch"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/ext/pluginspec"
	"reasonix/internal/ext/skill"
	"reasonix/internal/platform/lsp"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/safety/permission"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessiontemp"
	"reasonix/internal/tools/builtin"
)

// ErrUnknownModel is returned by Build when the configured model can't be
// resolved to a provider — e.g. a default_model left over from a renamed or
// removed provider. Callers can detect it (errors.Is) to re-run setup.
var ErrUnknownModel = provider.ErrUnknownModel

func agentKeepPolicy(keep []string) agent.KeepPolicy {
	if keep == nil {
		return agent.KeepErrors | agent.KeepUserMarked
	}
	var p agent.KeepPolicy
	for _, k := range keep {
		switch strings.TrimSpace(k) {
		case "errors":
			p |= agent.KeepErrors
		case "user_marked":
			p |= agent.KeepUserMarked
		}
	}
	return p
}

func recoveryHeadlessMode(opts Options) bool {
	return strings.TrimSpace(opts.HeadlessApprovalMode) != ""
}

// build is the assembly body behind BuildRuntime (and the Build compat
// wrapper): it loads config, resolves the model(s), wires the full runtime,
// and freezes the extension kernel snapshot from the objects it just
// assembled. The returned controller owns plugin subprocesses; call Close
// (via Controller.Close) to release them.
func build(ctx context.Context, opts Options) (*BuildResult, error) {
	timer := newPhaseTimer()
	ctx, opts, owner, fileWriteReceipt := bindRuntimeOwner(ctx, opts)
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	root, roots := resolveWorkspaceRoot(opts.WorkspaceRoot), opts.roots()
	additionalDirs, err := normalizeAdditionalDirs(root, opts.AdditionalDirs)
	if err != nil {
		return nil, err
	}
	migrations := runConfigMigrations(roots, root)
	cfg, err := roots.LoadForRoot(root)
	if err != nil {
		return nil, err
	}
	migrations.deepSeekErr = deepSeekProtocolMigrationNoticeError(handleConfigLoadWarnings(opts, cfg, stderr), migrations.deepSeekErr)
	// Arm the credential-protection layers from the user-global [secrets]
	// section before any tool, hook, or plugin subprocess can spawn. Package
	// globals are correct here because [secrets] is user-global (project
	// reasonix.toml cannot override it), so concurrent workspaces agree.
	secrets.SetFilterSubprocessEnv(cfg.Secrets.FilterSubprocessEnv)
	secrets.SetProtectSensitiveFiles(cfg.Secrets.ProtectSensitiveFiles)
	secrets.SetProtectCredentialFiles(cfg.Secrets.ProtectCredentialFiles)
	secrets.RegisterCredentialEnvKeys(cfg.CredentialEnvNames())
	// One synchronized sink for every emitter: background jobs and sidecars
	// emit from their own goroutines. The goal tee and the coalescer wrap it
	// here, before the extension hub captures it, so agents emit through both.
	sink := control.NewGoalUsageTee(event.Coalesce(quotedSink(cfg, opts), event.DefaultStreamDeltaWindow))

	proxySpec := cfg.NetworkProxySpec()
	ext, err := startExtensions(ctx, opts, roots, root, owner, sink)
	if err != nil {
		return nil, err
	}
	timer.mark("extensions")
	// Until assembly takes ownership of the sidecars, every error path retires them.
	pendingMgr := ext.mgr
	defer func() {
		if pendingMgr != nil {
			close(ext.failed)
			_ = pendingMgr.Close()
		}
	}()
	providers, err := resolveProviders(opts, cfg, proxySpec, ext.mgr, owner)
	if err != nil {
		return nil, err
	}
	baseResolver, effectiveResolver, extensionResolver := providers.base, providers.effective, providers.extension
	extensionMgr, extWarn, extUIHub, generation, sessionID := ext.mgr, ext.warn, ext.hub, ext.generation, ext.sessionID

	model, err := selectModel(opts, cfg, extensionResolver)
	if err != nil {
		return nil, err
	}
	modelName, modelRef, entry := model.name, model.ref, model.entry
	agentPreset, tokenDelivery, runtimeProfile := model.preset, model.delivery, model.profile
	keepPolicy := agentKeepPolicy(cfg.Agent.Keep)

	migrations.report(sink, cfg)
	timer.mark("config")
	migrateLegacySources(opts, sink)
	timer.mark("migrations")
	if ignored := cfg.IgnoredProjectDefaultModel(); ignored != "" {
		report(sink, event.Event{Level: event.LevelWarn, Text: "Ignored the project config's default_model.", Detail: fmt.Sprintf("./reasonix.toml sets default_model = %q but no configured provider serves it; using %q from your user config instead. Edit or remove that default_model line to silence this notice.", ignored, cfg.DefaultModel)})
	}

	// A resolvable model whose API key env is unset would otherwise build fine
	// (RequireKey is false so the UI stays reachable) and then fail silently on the
	// first request, showing as an empty/dead model. Surface the cause up front.
	if !opts.RequireKey && entry.RequiresAPIKey() && entry.APIKey() == "" {
		report(sink, event.Event{Text: "Selected model is missing its API key.", Detail: fmt.Sprintf("model %q is selected but its API key %s is not set — requests will fail until you set it", modelName, entry.APIKeyEnv)})
	}
	session, err := startSessionRuntime(opts, cfg, root, sink)
	if err != nil {
		return nil, err
	}
	timer.mark("sessions")

	// Validated before the first provider is constructed.
	if err := netclient.Validate(proxySpec); err != nil {
		return nil, err
	}
	balanceClient, err := netclient.NewHTTPClient(proxySpec, netclient.TransportOptions{})
	if err != nil {
		return nil, err
	}
	execProv, err := resolveProvider(effectiveResolver, cfg, proxySpec, provider.Selection{Ref: modelRef, Effort: opts.EffortOverride})
	if err != nil {
		return nil, err
	}
	timer.mark("provider")
	shell := sandbox.ResolveShell(cfg.Tools.Shell.Prefer, cfg.Tools.Shell.Path, stderr)

	prompt, err := buildPromptAssembly(ctx, opts, cfg, root, shell, sink, timer)
	if err != nil {
		return nil, err
	}

	reg := tool.NewRegistry()
	env := resolveToolEnvironment(opts, cfg, roots, root, additionalDirs, shell, stderr)
	writeRoots, forbidReadRoots, networkEnabled := env.writeRoots, env.forbidReadRoots, env.network
	bashSpec, readPathResolver, sessionTemp := env.bash, env.readPaths, env.sessionTemp
	enabledBuiltins := cfg.Tools.Enabled
	// The full inventory registers for use_capability; the provider-visible surface narrows later.
	addBuiltins(reg, enabledBuiltins, writeRoots, bashSpec, env.bashTimeout, env.search, stderr, root, proxySpec, forbidReadRoots, readPathResolver, env.sessionGuard, env.managedConfig, opts.FileOverlay, opts.TerminalRunner, sessionTemp, fileWriteReceipt)
	addSystemOne(reg, enabledBuiltins, cfg, balanceClient)
	// Use the caller-supplied shared host when set, so controllers for the same
	// workspace root reuse running MCP processes (e.g. one CodeGraph daemon
	// instead of one per tab). Otherwise construct a private host per controller.
	pluginHost := opts.SharedHost
	if pluginHost == nil {
		pluginHost = plugin.NewHost()
	}
	// Where the host reports that a server's state changed. Without it a lazy
	// server that connects in the background — the common case, since a
	// cache-miss server is started by its first real tool call — leaves every
	// status view showing whatever it saw at boot.
	pluginHost.SetStatusSink(opts.Sink)

	// Enabled MCP servers enter the tool catalog at boot. Cached schemas
	// register placeholders without starting processes; cache-miss servers get
	// a single background catalog discovery. First real tool call uses
	// EnsureConnected so parent/child/tab runtimes share one process.
	pluginSpecOptions := pluginspec.Options{
		DefaultStartupTimeout: time.Duration(cfg.MCPStartupTimeoutSeconds()) * time.Second,
		DefaultCallTimeout:    time.Duration(cfg.MCPCallTimeoutSeconds()) * time.Second,
		LaunchManager:         mcplaunch.ForWorkspace(roots.Home(), root),
		ConfigSource:          "workspace_config",
		StateHome:             roots.Home(),
		WriterRoots:           writeRoots,
		ForbidReadRoots:       forbidReadRoots,
		Network:               networkEnabled,
		PackageOwners:         pluginspec.PackageOwners(cfg),
		OAuthHTTPClient:       balanceClient,
	}
	mcp := resolveMCPSpecs(opts, cfg, root, pluginSpecOptions)

	configSpecs := registerMCPTools(ctx, pluginHost, reg, mcp, sink)

	cleanup := pluginHost.Close
	if opts.SharedHost != nil {
		// The caller owns the shared host's lifecycle; the controller must not
		// close it. A no-op cleanup keeps Controller.Close happy without
		// shutting down MCP processes that other controllers still use.
		cleanup = func() {}
	}

	// The LSP manager is session-scoped: its servers stop with the controller.
	lspMgr := registerLSP(reg, cfg, root)
	if lspMgr != nil {
		prev := cleanup
		cleanup = func() { prev(); lspMgr.Close() }
	}
	browserSession := bindMachineTools(reg, cfg.Browser, root, opts.BrowserSession)

	timer.mark("mcp")
	maxSteps := max(opts.MaxSteps, 0)
	subagentStore, err := newSubagentStore(session.dir, opts.SubagentParentLive)
	if err != nil {
		return nil, err
	}
	if subagentStore != nil {
		subagentStore.WithDestroyedChecker(session.jobs.IsDestroying)
	}

	// Permission policy gates every tool call. With no HeadlessApprovalMode
	// (interactive bootstrap), the temporary gate preserves the legacy behavior
	// until chat/desktop installs an interactive gate. A real headless caller
	// such as `reasonix run` always supplies a mode: Ask fails closed, Auto
	// allows ordinary writer fallbacks, and DontAsk denies them (#6927).
	// The selected contract is also applied to sub-agents, so they cannot be a
	// weaker path around the parent gate.
	// Sub-agents always run headless: they have no UI to answer a prompt, so they
	// inherit this same gate.
	policy := permission.New(cfg.Permissions.Mode, cfg.Permissions.Allow, cfg.Permissions.Ask, cfg.Permissions.Deny).
		WithAllowDynamicBashFallback(cfg.Permissions.AllowDynamicBash).
		WithSessionAllow(opts.PermissionAllow)
	headlessGate := control.NewSharedHeadlessGate(policy, opts.HeadlessApprovalMode)

	resolvedHooks, hookRunner := loadHooks(opts, roots, root, shell, sink)
	roles := roleWiring{cfg: cfg, roots: roots, resolver: effectiveResolver, extension: extensionResolver,
		proxy: proxySpec, sink: sink, gate: headlessGate, reg: reg, keep: keepPolicy}
	sub := newSubagentConfig(opts, cfg, entry, modelName, effectiveResolver, proxySpec, prompt.skillStore)
	taskTool, skillRun := roles.delegation(delegationInputs{opts: opts, sub: sub, exec: execProv, entry: entry,
		modelName: modelName, root: root, maxSteps: maxSteps, delivery: tokenDelivery, store: subagentStore,
		session: session, bashEnforced: bashSpec.Enforce})

	registerSessionTools(reg, opts.Ablation, roots, session.dir, prompt.memory.Store)

	readOnlySkillRunner, skillRunner, skillProfile := skillRun.runReadOnly, skillRun.run, skillProfile(cfg)
	cmds := loadCommands(opts, root)
	addInstallSourceTool(ctx, reg, pluginHost, root, balanceClient, pluginSpecOptions, opts.Stderr)
	registerSkillTools(reg, opts.Ablation, prompt.skillStore, prompt.implicitSkills,
		skillRunners{readOnly: readOnlySkillRunner, run: skillRunner, profile: skillProfile}, cmds)

	caps := newCapabilitySurface(ctx, cfg, root, pluginSpecOptions, pluginHost, reg, prompt.skillStore, runtimeProfile, mcp.enabled)
	capRuntime, capLedger, capAudit, capSpecs, catalogFn := caps.runtime, caps.ledger, caps.audit, caps.specs, caps.catalog
	skillRun.capRuntime = capRuntime
	addExtensionTools(reg, extensionMgr, extWarn)
	reg.Add(caps.proxy)
	gateSkillsOnCatalog(prompt.skillStore, catalogFn)

	execSess := newObservedSession(prompt.prompt)
	triageProv, triageRef, triagePrice := resolveTriage(cfg, modelRef, proxySpec)
	executor := agent.New(execProv, reg, execSess, agent.Options{
		MaxSteps:       maxSteps,
		MaxStepsKey:    opts.MaxStepsKey,
		Temperature:    cfg.Agent.Temperature,
		TaskBudget:     taskBudgetFromConfig(cfg),
		Pricing:        entry.Price,
		ModelRef:       modelRef,
		TriageProvider: triageProv, TriageModelRef: triageRef, TriagePricing: triagePrice,
		Gate:  headlessGate,
		Hooks: hookRunner,
		Jobs:  session.jobs,
		// Parent write reservation at the executor entry covers all writers
		// (including late Economy/MCP adds) without wrapping tool schemas.
		WriteScheduler:     sub.scheduler,
		WriteWorkspaceRoot: root, WorkspaceVCS: prompt.workspaceVCS, RenderRoot: renderRoot(browserSession, entry, root),
		ProjectChecks: prompt.projectChecks, ProjectSensitivePaths: prompt.sensitivePaths,
		AgentPreset:                  agentPreset,
		DeliveryProfile:              tokenDelivery,
		Ablation:                     opts.Ablation,
		WorkspaceLease:               session.lease,
		CapabilityLedger:             capLedger,
		CapabilityAudit:              capAudit,
		ContextWindow:                entry.ContextWindow,
		CompactRatio:                 cfg.Agent.CompactRatio,
		ContextEditing:               cfg.Agent.ContextEditing,
		RecentKeep:                   cfg.Agent.RecentKeep,
		CompactionBudgets:            compactionBudgets(cfg),
		ArchiveDir:                   roots.ArchiveDir(),
		KeepPolicy:                   keepPolicy,
		ReasoningLanguage:            cfg.ReasoningLanguage(),
		SubagentDepth:                0,
		MaxSubagentDepth:             sub.maxDepth,
		MissingReasoningWarnStateDir: config.MissingReasoningWarnStateDir(),
	}, sink)

	runner, label, err := roles.planner(opts, executor, entry.Model, prompt.memory.StaticContext(), capRuntime)
	if err != nil {
		return nil, err
	}

	ctrlOpts := control.Options{
		TaskBudget:                     taskBudgetFromConfig(cfg),
		GoalTokenBudget:                cfg.Agent.GoalTokenBudget,
		Runner:                         runner,
		Executor:                       executor,
		Sink:                           sink,
		Policy:                         policy,
		SubagentGate:                   headlessGate,
		Label:                          label,
		ModelRef:                       modelRef,
		SystemPrompt:                   prompt.prompt,
		SessionDir:                     session.dir,
		Host:                           pluginHost,
		Commands:                       cmds,
		Skills:                         prompt.skills,
		AllSkills:                      prompt.allSkills,
		SkillStore:                     prompt.skillStore,
		AllSkillStore:                  prompt.allSkillStore,
		DisableImplicitSkillInvocation: !prompt.implicitSkills,
		SkillRunner:                    skillRunner,
		ReadOnlySkillRunner:            readOnlySkillRunner,
		SkillProfile:                   skillProfile,
		Hooks:                          hookRunner,
		Memory:                         prompt.memory,
		// Indirection: the cleanup variable gains the extension runtime set at
		// the end of build (snapshot assembly runs after control.New), and the
		// controller must observe the final chain at Close time.
		Cleanup:               func() { cleanup() },
		Balance:               opts.BalanceStore.Cache(balanceClient, entry.BalanceURL, entry.APIKey()),
		Jobs:                  session.jobs,
		TaskStore:             opts.TaskStore,
		WorkspaceLease:        session.lease,
		Registry:              reg,
		PluginCtx:             ctx,
		MCPDefaultCallTimeout: pluginSpecOptions.DefaultCallTimeout,
		MCPConfigureSpec: func(spec *plugin.Spec) {
			if spec == nil {
				return
			}
			spec.LaunchManager = pluginSpecOptions.LaunchManager
			if strings.TrimSpace(spec.ConfigSource) == "" {
				spec.ConfigSource = pluginSpecOptions.ConfigSource
			}
			if spec.DefaultStartupTimeout <= 0 {
				spec.DefaultStartupTimeout = pluginSpecOptions.DefaultStartupTimeout
			}
			pluginspec.ApplyIsolation(spec, root, pluginSpecOptions)
		},
		CapabilityRuntime:      capRuntime,
		WorkspaceRoot:          root,
		ExternalFolderToolRefs: readPathResolver,
		ResponseLanguage:       cfg.ResponseLanguage(),
		ReasoningLanguage:      cfg.ReasoningLanguage(),
		DisableColdResumePrune: !cfg.ColdResumePruneEnabled(),
		Shell:                  shell,
		ApprovalTimeout:        opts.ApprovalTimeout,
		RuntimeProfile:         runtimeProfile,
		Ablation:               opts.Ablation,
		OnRemember: func(rule string) control.RememberResult {
			return rememberPermissionRule(roots, root, rule)
		},
		SessionRecoveryMeta: opts.SessionRecoveryMeta,
		OnSessionRecovered:  opts.OnSessionRecovered,
		// The merged catalog (nil without provider-declaring sidecars) lets
		// frontends enumerate plugin/... models through ProviderCatalog.
		ProviderResolver:  extensionResolver,
		RuntimeGeneration: generation,
		RuntimeOwner:      owner,
		// Share the Manager already bound into bash/grep so tools and the
		// Controller observe the same temporary generation across rebuilds.
		SessionTemp:    sessionTemp,
		BrowserSession: browserSession,
	}
	ctrlOpts.Guardian = roles.guardian()
	if reviewer := roles.recoveryReviewer(modelRef); reviewer != nil {
		ctrlOpts.RecoveryReviewer = reviewer
	}
	// HeadlessApprovalMode declares the frontend has no decision channel;
	// ApprovalTimeout is not a proxy for that.
	ctrlOpts.RecoveryHeadless = recoveryHeadlessMode(opts)
	ctrlOpts.GoalEvaluator = goalEvaluator(cfg, modelRef, proxySpec, sink)
	ctrlOpts.PromptRefiner = promptRefiner(entry, proxySpec, sink)
	ctrl := withWindowPosture(control.New(ctrlOpts), cfg, opts.StatsSource)
	ext.publish(ctrl)
	// Share the recovery checkpoint with task/fleet sub-agents so background
	// writers observe the same failure state as the root agent.
	if taskTool != nil {
		if g := ctrl.Executor(); g != nil {
			taskTool.WithRecoveryGate(g.RecoveryGate())
		}
	}
	if capRuntime != nil {
		ctrl.SetCapabilityProxyTools(capRuntime.ConnectedProxyTools)
	}
	// The task tool was built before the capability runtime existed.
	if taskTool != nil && capRuntime != nil {
		taskTool.WithCapabilityRuntime(capRuntime)
	}
	router := roles.semanticRouter(sub, execProv, modelRef, entry.Price, capAudit)
	ctrl.WireCapabilityRouting(cfg.Plugins, capSpecs, router, capAudit)
	ctrl.SetCapabilityProxyRouting(true)

	// Provider-visible tool surface is identical for every role setting before
	// the extension snapshot freezes registry schemas for cache diagnostics.
	applyUnifiedProviderToolSurface(reg, opts.GoalTurnsUnreachable, opts.Ablation)

	// The snapshot freezes the objects this build wired; discovery never re-runs.
	// It records the base provider catalog: sidecar providers enter through the
	// manager's own contributions.
	mcpSpecs := enabledMCPSpecs(configSpecs, mcp.extra)
	snap, runtimeSet, extensionDispatcher, snapErr := assembleLegacySnapshot(ctx, legacyAssembly{
		systemPrompt: prompt.prompt,
		registry:     reg,
		skills:       prompt.skills,
		commands:     cmds,
		hooks:        resolvedHooks,
		mcpSpecs:     mcpSpecs,
		providers:    baseResolver.Catalog(),
	}, generation, extensionBoot{
		session:            protocol.SessionContext{SessionID: sessionID, WorkspaceRoot: root, Generation: generation},
		ui:                 extUIHub,
		onWarning:          extWarn,
		skipPromptStrategy: shouldSkipPromptStrategy(opts.PreviousPlan),
		previousDispatcher: opts.PreviousDispatcher,
	}, extensionMgr)
	// Ownership of the preflighted Manager transferred to assembly on every
	// path: it was either closed inside or registered into the RuntimeSet.
	pendingMgr = nil
	if snapErr != nil {
		if extensionContractBroken(snapErr) {
			ctrl.ReleaseResources()
			return nil, fmt.Errorf("boot: %w", snapErr)
		}
		slog.Warn("boot: extension snapshot assembly failed; continuing without a runtime snapshot", "err", snapErr)
		runtimeSet = extension.NewRuntimeSet(generation)
		// Assembly retired the preflighted Manager on the error path; the
		// controller must not bind a hub or expose a manager whose sidecars
		// are already shut down.
		extensionMgr = nil
	}
	providerResolver := baseResolver
	if extensionResolver != nil {
		providerResolver = extensionResolver
	}
	cleanup = wireRuntimeScopeCleanup(runtimeSet, cleanup, opts.SharedHost, pluginHost, lspMgr, opts.SessionTemp)
	ctrl.SetExtensions(extensionDispatcher)
	if extensionMgr == nil {
		extUIHub = nil
	} else {
		ctrl.SetExtensionUI(extUIHub)
	}
	if providerResolver != nil {
		ctrl.SetProviderResolver(providerResolver)
	}
	// A prompt strategy may have replaced the prompt the executor session was
	// built with; swap it in before any turn so session and snapshot agree.
	if snap != nil {
		if final := snap.SystemPrompt(); final != prompt.prompt {
			ctrl.ApplyExtensionSystemPrompt(final)
		}
	}
	assembly := &ReusedAssembly{
		SystemPrompt:            prompt.prompt,
		Skills:                  prompt.skills,
		Commands:                cmds,
		Hooks:                   resolvedHooks,
		Registry:                reg,
		ImplicitSkillInvocation: prompt.implicitSkills,
		Memory:                  prompt.memory,
		ProjectChecks:           prompt.projectChecks, ProjectSensitivePaths: prompt.sensitivePaths,
	}
	return finalizeBuildResult(roots, &BuildResult{Controller: ctrl, Snapshot: snap, Runtime: runtimeSet, Owner: owner, Extensions: extensionMgr, Dispatcher: extensionDispatcher, ExtensionUI: extUIHub, ProviderResolver: providerResolver, BaseProviderResolver: baseResolver, Assembly: assembly, Phases: timer.done("assemble")}, !opts.deferPublish), nil
}

// effectivePlannerModel centralizes planner precedence. Every role setting
// builds the configured planner so later in-place switches retain the same
// runtime; the per-turn TaskPolicy decides whether it is invoked.
func effectivePlannerModel(cfg *config.Config, opts Options) string {
	if cfg == nil || opts.Ablation.Off(ablation.Planner) {
		return ""
	}
	return strings.TrimSpace(cfg.Agent.PlannerModel)
}

func rememberPermissionRule(roots config.Roots, workspaceRoot, rule string) control.RememberResult {
	path := rememberPermissionConfigPath(roots, workspaceRoot)
	result := control.RememberResult{Rule: strings.TrimSpace(rule), Path: path}
	unlock, err := config.LockConfigFileEdits(path)
	if err != nil {
		slog.Warn("lock config for permission rule", "path", path, "err", err)
		result.Err = err
		return result
	}
	defer unlock()

	edit, err := config.LoadForEditReadOnlyStrict(path)
	if err != nil {
		slog.Warn("load config for permission rule", "path", path, "err", err)
		result.Err = err
		return result
	}
	if coveredBy := coveredPermissionRule(edit.Permissions.Allow, result.Rule); coveredBy != "" {
		result.CoveredBy = coveredBy
		return result
	}
	edit.Permissions.Allow = pruneCoveredPermissionRules(edit.Permissions.Allow, result.Rule)
	if err := edit.AddPermissionRule("allow", rule); err != nil {
		slog.Warn("persist permission rule", "rule", rule, "err", err)
		result.Err = err
		return result
	}
	if err := config.WritePermissionsAllow(path, edit.Permissions.Allow); err != nil {
		slog.Warn("save config after permission rule", "err", err)
		result.Err = err
		return result
	}
	result.Saved = true
	return result
}

func rememberPermissionConfigPath(roots config.Roots, workspaceRoot string) string {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot != "" {
		return filepath.Join(workspaceRoot, "reasonix.toml")
	}
	path := roots.SourcePathForRoot(".")
	if path == "" {
		path = "reasonix.toml" // match Config.Save() fallback
	}
	return path
}

func coveredPermissionRule(rules []string, rule string) string {
	for _, existing := range rules {
		if permission.RuleCoversString(existing, rule) {
			return strings.TrimSpace(existing)
		}
	}
	return ""
}

func pruneCoveredPermissionRules(rules []string, rule string) []string {
	out := rules[:0]
	for _, existing := range rules {
		if strings.TrimSpace(existing) == "" || permission.RuleCoversString(rule, existing) {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func subagentModelRef(cfg *config.Config, sk skill.Skill) string {
	if cfg != nil {
		for _, key := range SubagentModelKeys(sk.Name) {
			if m := strings.TrimSpace(cfg.Agent.SubagentModels[key]); m != "" {
				return m
			}
		}
	}
	if m := strings.TrimSpace(sk.Model); m != "" {
		return m
	}
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Agent.SubagentModel)
}

func subagentEffortRef(cfg *config.Config, sk skill.Skill) string {
	if cfg != nil {
		for _, key := range SubagentModelKeys(sk.Name) {
			if e := strings.TrimSpace(cfg.Agent.SubagentEfforts[key]); e != "" {
				return e
			}
		}
	}
	if e := strings.TrimSpace(sk.Effort); e != "" {
		return e
	}
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Agent.SubagentEffort)
}

// SubagentModelKeys returns the cfg.Agent.SubagentModels/SubagentEfforts map
// keys that resolve for a subagent name, in precedence order: the exact name
// first, then its underscore/hyphen alias variants (the dedicated tool
// security_review dispatches the skill security-review, so either spelling in
// config must reach it). Any surface that reads OR clears these maps must
// iterate this same key set — an exact-key delete leaves an alias entry
// silently active.
func SubagentModelKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	keys := []string{name}
	for _, alias := range []string{
		strings.ReplaceAll(name, "-", "_"),
		strings.ReplaceAll(name, "_", "-"),
	} {
		if alias == "" {
			continue
		}
		seen := slices.Contains(keys, alias)
		if !seen {
			keys = append(keys, alias)
		}
	}
	return keys
}

func resolveWorkspaceRoot(explicit string) string {
	if explicit != "" {
		return explicit
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if root, ok := nearestGitRoot(wd); ok {
		return root
	}
	return wd
}

func normalizeAdditionalDirs(root string, dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		return nil, nil
	}
	base := strings.TrimSpace(root)
	if base == "" {
		base = "."
	}
	if !filepath.IsAbs(base) {
		abs, err := filepath.Abs(base)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace root: %w", err)
		}
		base = abs
	}

	var out []string
	for _, raw := range dirs {
		dir := strings.TrimSpace(raw)
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(base, dir)
		}
		dir, err := filepath.Abs(filepath.Clean(dir))
		if err != nil {
			return nil, fmt.Errorf("resolve additional directory %q: %w", raw, err)
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve additional directory %q: %w", raw, err)
		}
		info, err := os.Stat(real)
		if err != nil {
			return nil, fmt.Errorf("inspect additional directory %q: %w", raw, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("additional path %q is not a directory", raw)
		}
		out = appendUniquePaths(out, filepath.Clean(real))
	}
	return out, nil
}

func appendUniquePaths(base []string, extra ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(out)+len(extra))
	for _, path := range out {
		seen[pathComparisonKey(path)] = struct{}{}
	}
	for _, path := range extra {
		path = filepath.Clean(path)
		key := pathComparisonKey(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}
	return out
}

// RuntimeForbidReadRoots returns the configured deny roots plus every path the
// secrets package denies readers: Reasonix's own credential FILE, the host's
// SSH private keys and cloud credential files, and the broad denylist's
// directories when it is on. It also registers the corresponding credential
// environment names for subprocess filtering. Runtime tool assemblers outside
// Build must use this helper instead of reading the config roots directly.
//
// These roots are what reaches the OS sandbox, which is where a protection
// stops being advisory: a denylist only the in-process readers consult leaves
// `cat` reading what read_file refuses.
func RuntimeForbidReadRoots(cfg *config.Config, root string) []string {
	if cfg == nil {
		return nil
	}
	secrets.RegisterCredentialEnvKeys(cfg.CredentialEnvNames())
	base := cfg.ForbidReadRootsForRoot(root)
	base = appendUniquePaths(base, secrets.ForbiddenReadPaths(cfg.Secrets.ProtectSensitiveFiles)...)
	credentialPath := strings.TrimSpace(cfg.Roots().UserCredentialsPath())
	if credentialPath == "" {
		return append([]string(nil), base...)
	}
	info, err := os.Stat(credentialPath)
	if err != nil || info.IsDir() {
		return append([]string(nil), base...)
	}
	if real, err := filepath.EvalSymlinks(credentialPath); err == nil {
		credentialPath = real
	}
	return appendUniquePaths(base, credentialPath)
}

func pathComparisonKey(path string) string {
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func nearestGitRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = filepath.Clean(start)
	}
	for {
		if isGitMarker(filepath.Join(dir, ".git")) {
			return dir, true
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", false
		}
		dir = next
	}
}

func isGitMarker(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && (fi.IsDir() || fi.Mode().IsRegular())
}

func newSubagentStore(sessionDir string, parentLive func(sessionPath string) bool) (*agent.SubagentStore, error) {
	sessionDir = strings.TrimSpace(sessionDir)
	if sessionDir == "" {
		return nil, nil
	}
	store := agent.NewSubagentStore(filepath.Join(sessionDir, "subagents")).WithParentSessionProbe(parentLive)
	if _, err := store.CleanupStaleRunning(); err != nil {
		return nil, fmt.Errorf("cleanup stale subagents: %w", err)
	}
	return store, nil
}

func subagentEffectiveIdentity(cfg *config.Config, resolver provider.Resolver, baseModelRef string, base *config.ProviderEntry, modelRef, effort string) (string, string) {
	var entry config.ProviderEntry
	if base != nil {
		entry = *base
	}
	ref := strings.TrimSpace(modelRef)
	explicit := ref != ""
	if !explicit {
		ref = strings.TrimSpace(baseModelRef)
	}
	if explicit && cfg != nil && ref != "" {
		if resolved, ok := cfg.ResolveModel(ref); ok {
			entry = *resolved
		} else if resolved := syntheticEntryFromResolver(resolver, ref); strings.TrimSpace(resolved.Name) != "" {
			entry = *resolved
		} else {
			entry.Model = ref
		}
	} else if explicit {
		if resolved := syntheticEntryFromResolver(resolver, ref); strings.TrimSpace(resolved.Name) != "" {
			entry = *resolved
		} else {
			entry.Model = ref
		}
	} else if base == nil && ref != "" {
		if resolved := syntheticEntryFromResolver(resolver, ref); strings.TrimSpace(resolved.Name) != "" {
			entry = *resolved
		} else if cfg != nil {
			if resolved, ok := cfg.ResolveModel(ref); ok {
				entry = *resolved
			}
		}
	}
	if rawEffort := strings.TrimSpace(effort); rawEffort != "" {
		if normalized, err := config.NormalizeEffort(&entry, rawEffort); err == nil {
			entry.Effort = normalized
		} else {
			entry.Effort = rawEffort
		}
	}
	modelID := strings.TrimSpace(entry.Name)
	model := strings.TrimSpace(entry.Model)
	if modelID != "" && model != "" {
		modelID += "/" + model
	} else if model != "" {
		modelID = model
	} else if modelID == "" {
		modelID = ref
	}
	return modelID, strings.TrimSpace(config.EffectiveEffort(&entry))
}

// addBuiltins adds enabled built-in tools to reg. An empty list means all of
// them. writeRoots confines the file-writing built-ins to the workspace: after
// the (unconfined) defaults are added, each enabled writer is replaced by an
// instance bound to writeRoots (preserving registry order).
// forbidReadRoots confines the read/list/search built-ins so they cannot peek at
// the listed directories.
// When workDir is non-empty, tools resolve relative paths against it instead of
// the process cwd, enabling concurrent multi-project sessions.
// sessionGuard blocks writer-tool targets inside Reasonix's own session stores
// and makes bash warn when a command references them. managedConfig names the
// Reasonix-owned config files writable outside writeRoots after a fresh
// per-write human approval.
func addBuiltins(reg *tool.Registry, enabled, writeRoots []string, bashSpec sandbox.Spec, bashTimeout time.Duration, searchSpec builtin.SearchSpec, stderr io.Writer, workDir string, proxySpec netclient.ProxySpec, forbidReadRoots []string, readPathResolver *builtin.PathResolver, sessionGuard builtin.SessionDataGuard, managedConfig builtin.ManagedConfigPaths, overlay builtin.FileOverlay, terminal builtin.TerminalRunner, sessionTemp *sessiontemp.Manager, fileWriteReceipt func(path string, hadPrior bool, prior []byte)) {
	// If a workspace directory is set, use workspace-bound tools that resolve
	// paths relative to that directory. Otherwise fall back to the process-cwd
	// compile-time builtins.
	if workDir != "" {
		ws := builtin.Workspace{Dir: workDir, WriteRoots: writeRoots, ForbidReadRoots: forbidReadRoots, Bash: bashSpec, BashTimeout: bashTimeout, Search: searchSpec, ProxySpec: proxySpec, ReadPaths: readPathResolver, SessionGuard: sessionGuard, ManagedConfig: managedConfig, FileOverlay: overlay, Terminal: terminal, SessionTemp: sessionTemp, FileWriteReceipt: fileWriteReceipt}
		for _, t := range ws.Tools(enabled...) {
			reg.Add(t)
		}
		return
	}

	if len(enabled) == 0 {
		for _, t := range tool.Builtins() {
			reg.Add(t)
		}
	} else {
		for _, name := range enabled {
			if t, ok := tool.LookupBuiltin(name); ok {
				reg.Add(t)
			} else {
				fmt.Fprintf(stderr, "warning: unknown built-in tool %q\n", name)
			}
		}
	}
	// Replace the unconfined defaults with confined instances (registry order is
	// preserved on replace): file-writers bound to the workspace, read tools
	// bound to forbid-read roots, bash to the OS sandbox, web_fetch to the proxy.
	// Only replace tools actually enabled/present.
	bashTool := builtin.ConfineBash(bashSpec, sessionGuard, bashTimeout)
	if rebound, ok := builtin.BindSessionTemp(bashTool, sessionTemp); ok {
		bashTool = rebound
	}
	searchTool := builtin.ConfineSearch(searchSpec, bashSpec, forbidReadRoots)
	if rebound, ok := builtin.BindSessionTemp(searchTool, sessionTemp); ok {
		searchTool = rebound
	}
	writers := builtin.ConfineWriters(writeRoots, sessionGuard, managedConfig)
	for i, writer := range writers {
		if rebound, ok := builtin.BindSessionTemp(writer, sessionTemp); ok {
			writer = rebound
		}
		writers[i] = builtin.BindFileWriteReceipt(writer, fileWriteReceipt)
	}
	confined := append(writers,
		bashTool,
		searchTool,
		builtin.ConfineWebFetch(proxySpec))
	confined = append(confined, builtin.ConfineReaders(forbidReadRoots)...)
	for _, t := range confined {
		if _, ok := reg.Get(t.Name()); ok {
			reg.Add(t)
		}
	}
}

// autoShellPrefer reports whether [tools.shell] left the interpreter to
// auto-detection, so the "fell back to PowerShell" hint is suppressed once the
// user has explicitly chosen a shell.
func autoShellPrefer(prefer string) bool {
	p := strings.ToLower(strings.TrimSpace(prefer))
	return p == "" || p == "auto"
}

// LSPSpecs returns the language → server map: the built-in defaults overlaid with
// any user overrides. A user entry may set only the fields it wants to change;
// empty fields keep the default for that language.
func LSPSpecs(cfg config.LSPConfig) map[string]lsp.ServerSpec {
	specs := lsp.DefaultSpecs()
	for lang, s := range cfg.Servers {
		spec := specs[lang]
		if s.Command != "" {
			spec.Command = s.Command
		}
		if s.Args != nil {
			spec.Args = s.Args
		}
		if s.Env != nil {
			spec.Env = s.Env
		}
		if s.LanguageID != "" {
			spec.LanguageID = s.LanguageID
		}
		if s.Extensions != nil {
			spec.Extensions = s.Extensions
		}
		if s.InstallHint != "" {
			spec.InstallHint = s.InstallHint
		}
		if spec.LanguageID == "" {
			spec.LanguageID = lang
		}
		specs[lang] = spec
	}
	return specs
}

func providerNames(cfg *config.Config) string {
	names := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		names[i] = p.Name
	}
	return strings.Join(names, "/")
}
