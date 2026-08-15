package boot

import (
	"reasonix/internal/config"
	"reasonix/internal/instruction"
	"reasonix/internal/memory"
)

// memoryAssembly is the memory surface for one build: the live set the remember
// tools write through, the host checks its docs declare, and the prompt with
// the memory block folded in.
type memoryAssembly struct {
	set       *memory.Set
	checks    []instruction.VerifyCheck
	sysPrompt string
}

// buildMemoryAssembly loads memory and composes it into the prompt. A rebuild
// that reuses discovery skips the read: it also reuses the composed prompt, so
// re-reading the docs would only produce a block nothing goes on to use.
func buildMemoryAssembly(opts Options, cfg *config.Config, root, sysPrompt string) memoryAssembly {
	if opts.ReuseAssembly != nil && shouldReuseDiscovery(opts.PreviousPlan) && opts.ReuseAssembly.Memory != nil {
		return memoryAssembly{
			set:       opts.ReuseAssembly.Memory,
			checks:    opts.ReuseAssembly.ProjectChecks,
			sysPrompt: sysPrompt,
		}
	}
	set := memory.Load(memory.Options{
		CWD: root, UserDir: config.MemoryUserDir(),
		PinnedBudgetChars: cfg.Memory.PinnedBudgetChars,
		RecallLimit:       cfg.Memory.RecallLimit,
		RecallMaxChars:    cfg.Memory.RecallMaxChars,
	})
	return memoryAssembly{
		set:       set,
		checks:    instruction.ExtractHostChecks(set.Docs),
		sysPrompt: memory.Compose(sysPrompt, set),
	}
}
