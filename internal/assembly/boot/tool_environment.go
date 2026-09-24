package boot

import (
	"fmt"
	"io"
	"strings"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/state/sessiontemp"
	"reasonix/internal/tools/builtin"
)

// toolEnvironment is what confines the built-in tools: where they may write
// and read, the bash sandbox, and the session-scoped resources they share with
// the controller.
type toolEnvironment struct {
	writeRoots      []string
	forbidReadRoots []string
	network         bool
	bash            sandbox.Spec
	bashTimeout     time.Duration
	search          builtin.SearchSpec
	sessionGuard    builtin.SessionDataGuard
	managedConfig   builtin.ManagedConfigPaths
	readPaths       *builtin.PathResolver
	sessionTemp     *sessiontemp.Manager
}

func resolveToolEnvironment(opts Options, cfg *config.Config, roots config.Roots, root string, additionalDirs []string, shell sandbox.Shell, stderr io.Writer) toolEnvironment {
	env := toolEnvironment{
		forbidReadRoots: RuntimeForbidReadRoots(cfg, root),
		network:         cfg.Sandbox.Network,
		bashTimeout:     time.Duration(cfg.BashTimeoutSeconds()) * time.Second,
		readPaths:       builtin.NewPathResolver(),
		sessionTemp:     opts.SessionTemp,
	}
	env.writeRoots = appendUniquePaths(cfg.WriteRootsForRoot(root), additionalDirs...)
	if opts.WorkspaceOnly {
		env.writeRoots = []string{root}
	}
	if opts.SandboxNetworkOverride != nil {
		env.network = *opts.SandboxNetworkOverride
	}
	bashMode := cfg.BashMode()
	if override := strings.TrimSpace(opts.SandboxBashOverride); override != "" {
		bashMode = override
	}
	// Config repair outside the workspace goes through the approval-gated file
	// tools, never raw shell writes, so the bash write roots stay unwidened.
	env.managedConfig = builtin.NewManagedConfigPaths(config.ReasonixManagedConfigPaths())
	env.bash = sandbox.Spec{Mode: bashMode, WriteRoots: env.writeRoots, ForbidReadRoots: env.forbidReadRoots, Network: env.network,
		HostAuthorities: sandbox.ParseAuthorities(cfg.Sandbox.HostAuthorities), Shell: shell}
	// Agent writes into Reasonix's own session stores race the app's saves;
	// an explicit allow_write entry stays the sanctioned escape hatch.
	allowWriteRoots := cfg.AllowWriteRoots()
	if opts.WorkspaceOnly {
		allowWriteRoots = nil
	}
	env.sessionGuard = builtin.NewSessionDataGuard(roots.MemoryUserDir(), allowWriteRoots)
	if env.bash.Mode == "enforce" && !sandbox.Available() {
		fmt.Fprintln(stderr, "warning: "+sandbox.UnavailableMessage())
	}
	if autoShellPrefer(cfg.Tools.Shell.Prefer) && shell.Kind == sandbox.ShellPowerShell {
		fmt.Fprintln(stderr, "warning: bash not found on PATH; the shell tool will run commands under Windows PowerShell. Install Git for Windows or WSL to use bash, or set [tools.shell] prefer=\"powershell\" to silence this.")
	}
	env.search = builtin.ResolveSearch(cfg.Tools.Search.Engine, cfg.Tools.Search.RgPath, stderr)
	// A rebuild passes the previous controller's manager, so tools and
	// controller keep one temporary generation.
	if env.sessionTemp == nil {
		env.sessionTemp = sessiontemp.New()
	}
	return env
}
