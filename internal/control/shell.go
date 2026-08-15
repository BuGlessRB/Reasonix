package control

import (
	"fmt"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/sandbox"
	"reasonix/internal/shellrun"
	"reasonix/internal/tool"
)

// ShellOption is one interpreter this machine actually has. Path is what the
// probe found rather than a name to look up later, so a host carrying two of
// them offers two rows instead of one ambiguous "bash".
type ShellOption struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Version        string `json:"version,omitempty"`
	SupportsAndAnd bool   `json:"supportsAndAnd"`
	// Prefer is the value SaveShellSettings takes to select this option.
	Prefer string `json:"prefer"`
}

// ShellSettings is the shell tool's interpreter as an editor needs it: what is
// configured, what that resolved to, and what else is installed. Options are
// probed instead of listed from a fixed table — offering a shell the host does
// not have is a switch that breaks every command it accepts.
type ShellSettings struct {
	Prefer    string      `json:"prefer"`
	Path      string      `json:"path,omitempty"`
	Effective ShellOption `json:"effective"`
	// Auto is what detection picks here, so "自动" can name its own outcome.
	Auto     ShellOption   `json:"auto"`
	Options  []ShellOption `json:"options"`
	Platform string        `json:"platform"`
}

// ShellSettings reads the configured interpreter and everything installed
// beside it.
func (c *Controller) ShellSettings() ShellSettings {
	prefer, path := "auto", ""
	if cfg, err := config.Load(); err == nil {
		if p := strings.TrimSpace(cfg.Tools.Shell.Prefer); p != "" {
			prefer = strings.ToLower(p)
		}
		path = strings.TrimSpace(cfg.Tools.Shell.Path)
	}
	auto := sandbox.ResolveShell("", "", nil)
	effective := auto
	if prefer != "auto" || path != "" {
		effective = sandbox.ResolveShell(prefer, path, nil)
	}
	out := ShellSettings{
		Prefer:    prefer,
		Path:      path,
		Effective: shellOption(effective),
		Auto:      shellOption(auto),
		Platform:  shellrun.DescriptorFromShell(auto).Platform,
	}
	for _, sh := range sandbox.DetectShells() {
		out.Options = append(out.Options, shellOption(sh))
	}
	return out
}

// SaveShellSettings persists the interpreter choice after proving it runs: a
// path that cannot execute is refused on the screen that typed it rather than
// on every command afterwards. The caller rebuilds the runtime, because boot
// binds the interpreter into the shell tool while assembling it.
func (c *Controller) SaveShellSettings(prefer, path string) error {
	if err := sandbox.VerifyShell(prefer, path); err != nil {
		return fmt.Errorf("这个 shell 用不了：%w", err)
	}
	unlock := config.LockUserConfigEdits()
	defer unlock()
	cfg := config.LoadForEdit(config.UserConfigPath())
	if err := cfg.SetShell(prefer, path); err != nil {
		return err
	}
	return cfg.SaveTo(config.UserConfigPath())
}

func shellOption(sh sandbox.Shell) ShellOption {
	ex := shellrun.DescriptorFromShell(sh)
	opt := ShellOption{
		Name:           ex.Shell,
		Path:           sh.Path,
		Version:        ex.ShellVersion,
		SupportsAndAnd: ex.SupportsAndAnd,
		Prefer:         "bash",
	}
	switch ex.Shell {
	case tool.ShellNamePwsh:
		opt.Prefer = "pwsh"
	case tool.ShellNamePowerShell:
		opt.Prefer = "powershell"
	}
	return opt
}
