package boot

import (
	"reasonix/internal/contract/ablation"
	"reasonix/internal/contract/observe"
	"reasonix/internal/contract/tool"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/session/control"
)

// ObserveOptions builds an unattended, read-only run. It is a field of Options
// and nothing else: no flag, configuration key or environment variable reaches
// it, so only a host that assembles the run can ask for the posture.
type ObserveOptions struct {
	// Pending is where a request that needs a person is parked. Required.
	Pending observe.PendingSink
	// Run is what the run tells the model about itself on its turn tail.
	Run observe.RunContext
}

// observeOverrides pins every Options field the posture depends on. What the
// caller set for them is discarded, because a run that inherits a wider value
// is a run whose confinement depends on who called it.
func observeOverrides(opts Options) Options {
	if opts.Observe == nil {
		return opts
	}
	no := false
	opts.RuntimeReload = RuntimeReload{}
	opts.SharedHost = nil
	opts.ExtraPlugins = nil
	opts.AdditionalDirs = nil
	opts.PermissionAllow = nil
	opts.FileOverlay = nil
	opts.TerminalRunner = nil
	opts.HeadlessApprovalMode = control.ToolApprovalReadOnly
	opts.SandboxBashOverride = "enforce"
	opts.SandboxNetworkOverride = &no
	opts.WorkspaceOnly = true
	opts.GoalTurnsUnreachable = true
	opts.UnattendedChild = true
	opts.Ablation = ablation.New(ablation.Planner, ablation.Subagent)
	return opts
}

// newToolRegistry is the registry every tool of a build is added to. Under the
// posture it holds the ceiling from its first Add, so no registration path,
// however late, can put a tool the ceiling refuses in reach of the model.
func newToolRegistry(opts Options) *tool.Registry {
	reg := tool.NewRegistry()
	if opts.Observe != nil {
		reg.Restrict(observe.Admits)
	}
	return reg
}

func (b *builder) observeRun() *control.ObserveRun {
	if b.opts.Observe == nil {
		return nil
	}
	return &control.ObserveRun{
		Posture: observe.New(sandbox.IntegrityEnforced(b.tools.env.bash)),
		Pending: b.opts.Observe.Pending,
		Context: b.opts.Observe.Run,
	}
}
