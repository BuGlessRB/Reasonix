package boot

import (
	"context"
	"runtime"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/environment"
	"reasonix/internal/event"
	"reasonix/internal/instruction"
	"reasonix/internal/memory"
	"reasonix/internal/outputstyle"
	"reasonix/internal/sandbox"
	"reasonix/internal/skill"
)

// promptAssembly is the cache-stable prefix a build composes once, together
// with what the memory and skill folds produced along the way. Nothing here may
// be rewritten mid-session: DeepSeek's prefix cache keys on these exact bytes,
// so later changes ride the turn tail instead (see control.Compose).
type promptAssembly struct {
	prompt         string
	memory         *memory.Set
	projectChecks  []instruction.VerifyCheck
	sensitivePaths []string
	skillStore     *skill.Store
	allSkillStore  *skill.Store
	skills         []skill.Skill
	allSkills      []skill.Skill
	implicitSkills bool
}

// buildPromptAssembly resolves the base system prompt and folds in, in order,
// the output style, core policies, workspace line, environment section, offline
// note, memory, and the skill index. Order is the contract: it decides the
// prefix bytes every turn reuses.
func buildPromptAssembly(ctx context.Context, opts Options, cfg *config.Config, root string, shell sandbox.Shell, sink event.Sink, timer *phaseTimer) (promptAssembly, error) {
	sysPrompt, err := cfg.ResolveSystemPromptForRoot(root)
	if err != nil {
		if !config.IsMissingSystemPromptFile(err) {
			return promptAssembly{}, err
		}
		// A stale missing prompt file must not block startup: warn and fall back
		// to the inline (or built-in default) system prompt. Other read failures
		// stay fatal so Reasonix never runs without explicitly configured policy.
		report(sink, event.Event{Level: event.LevelWarn, Text: err.Error() + "; falling back to inline/default system prompt"})
		sysPrompt = cfg.InlineSystemPrompt()
	}
	// Output style: fold the selected persona/tone block into the base prompt
	// before language/memory/skills append, so a "replace" style (keep-coding
	// false) still keeps those. Applied once, into the cache-stable prefix.
	if st, ok := outputstyle.Resolve(cfg.Agent.OutputStyle, outputstyle.Dirs()); ok {
		sysPrompt = outputstyle.Apply(sysPrompt, st)
	}
	sysPrompt = appendCorePolicies(sysPrompt)
	// Role settings and the workspace path both ride the per-turn transient user
	// blocks instead of landing here, so this prefix is identical for every
	// project and session on the machine and they share one cache entry.
	if cfg.EnvironmentEnabled() {
		shellLabel := shell.Kind.String()
		if strings.TrimSpace(cfg.Tools.Shell.Path) != "" {
			shellLabel = shell.Path
		}
		envSection := environment.FormatSection(
			environment.RunProbesWithOptions(ctx, environment.DefaultProbes(), environment.ProbeOptions{
				Overrides: cfg.Environment.Tools,
				DenyRoots: []string{root},
				// This section sits inside the cached prefix, so re-probing
				// per boot let transient flaps (timeouts, PATH drift) rewrite
				// it and cold-start every session's cache.
				SnapshotDir: opts.roots().CacheDir(),
			}),
			runtime.GOOS+"/"+runtime.GOARCH,
			shellLabel,
			environment.WorkspaceVCS(root),
			cfg.Environment.Tools,
		)
		if envSection != "" {
			sysPrompt += "\n\n" + envSection
		}
	}
	sysPrompt = appendOfflineEnvironmentNote(sysPrompt, cfg.Environment.Offline)

	// Memory folds in exactly here, once, becoming part of the durable prefix,
	// so it costs nothing per turn. Mid-session changes ride the controller's
	// transient turn-injection and fold in on the next session instead.
	if !continuesGeneration(opts) {
		if _, err := memory.StoreFor(opts.roots().MemoryUserDir(), root).MigrateV2(); err != nil {
			report(sink, event.Event{Level: event.LevelWarn, Text: "Memory metadata migration did not complete.", Detail: err.Error()})
		}
	}
	memSet := buildMemoryAssembly(opts, cfg, root, sysPrompt)
	timer.mark("memory")

	implicit := cfg.ImplicitSkillInvocationEnabled()
	skillSet := buildSkillAssembly(opts, cfg, root, implicit, memSet.sysPrompt)
	timer.mark("skills")

	return promptAssembly{
		prompt:         skillSet.sysPrompt,
		memory:         memSet.set,
		projectChecks:  memSet.checks,
		sensitivePaths: memSet.sensitive,
		skillStore:     skillSet.store,
		allSkillStore:  skillSet.all,
		skills:         skillSet.skills,
		allSkills:      skillSet.allSkills,
		implicitSkills: implicit,
	}, nil
}
