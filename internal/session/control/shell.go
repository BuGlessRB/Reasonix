package control

import (
	"context"
	"fmt"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/tools/shellrun"
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
	out.Options = oneGitBashPerInstall(out.Options, out.Effective.Path)
	return out
}

// oneGitBashPerInstall keeps a single row per Git for Windows install. Its
// bin/bash.exe is a launcher for usr/bin/bash.exe, so offering both draws two
// "Git Bash" buttons that run the same program. The row matching keep wins, so
// a pinned path still shows as selected.
func oneGitBashPerInstall(opts []ShellOption, keep string) []ShellOption {
	at := map[string]int{}
	out := opts[:0:0]
	for _, o := range opts {
		if o.Name != tool.ShellNameGitBash {
			out = append(out, o)
			continue
		}
		root := gitInstallRoot(o.Path)
		i, seen := at[root]
		if !seen {
			at[root] = len(out)
			out = append(out, o)
			continue
		}
		if strings.EqualFold(o.Path, keep) {
			out[i] = o
		}
	}
	return out
}

func gitInstallRoot(path string) string {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	for _, tail := range []string{"/usr/bin/bash.exe", "/bin/bash.exe", "/usr/bin/bash", "/bin/bash"} {
		if root, ok := strings.CutSuffix(p, tail); ok {
			return root
		}
	}
	return p
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

// shellContextBytes bounds how much of a user command's output goes into the
// conversation; the rest is cut from the middle and the cut is said.
const shellContextBytes = 24 << 10

// answerShell puts a command the user ran into the conversation and lets the
// model respond to it, the way a line typed to the agent would be. A command
// the user stopped is not followed by a turn, and neither is one run with no
// model to answer it.
func (c *Controller) answerShell(ctx context.Context, command, state string, exit *int, output, errText string) error {
	if c.runner == nil || state == tool.ShellStateCancelled {
		return nil
	}
	input := shellTurnInput(command, exit, output, errText)
	return c.runTurnLoop(ctx, orchestratedTurn{input: input, raw: input, display: "!" + command})
}

func shellTurnInput(command string, exit *int, output, errText string) string {
	var b strings.Builder
	b.WriteString("I ran this command in my terminal:\n<bash-input>" + command + "</bash-input>\n")
	if exit != nil {
		fmt.Fprintf(&b, "<bash-exit-code>%d</bash-exit-code>\n", *exit)
	}
	if errText != "" {
		b.WriteString("<bash-error>" + errText + "</bash-error>\n")
	}
	if len(output) > shellContextBytes {
		half := shellContextBytes / 2
		head, tail := strings.ToValidUTF8(output[:half], ""), strings.ToValidUTF8(output[len(output)-half:], "")
		output = fmt.Sprintf("%s\n[… %d bytes of output cut from the middle …]\n%s", head, len(output)-2*half, tail)
	}
	b.WriteString("<bash-output>\n" + output + "\n</bash-output>")
	return b.String()
}
