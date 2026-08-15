import type { ShellOption, ShellSettings } from "./port";
import { MockExtensions } from "./mock_ext";

// The interpreter half of the fixture. It stands on a Windows host because that
// is the only interesting shape: three shells installed, two of them PowerShell,
// and which one gets picked decides whether the agent may write '&&' at all.
// Chained onto MockExtensions for the reason given there — MockPort satisfies
// AgentPort in one declaration, and each face keeps its own file.
const GIT_BASH: ShellOption = {
  name: "git-bash",
  path: "C:\\Program Files\\Git\\bin\\bash.exe",
  supportsAndAnd: true,
  prefer: "bash",
};

const PWSH: ShellOption = {
  name: "pwsh",
  path: "C:\\Program Files\\PowerShell\\7\\pwsh.exe",
  version: "7+",
  supportsAndAnd: true,
  prefer: "pwsh",
};

const POWERSHELL: ShellOption = {
  name: "powershell",
  path: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
  version: "5.1",
  supportsAndAnd: false,
  prefer: "powershell",
};

export class MockShell extends MockExtensions {
  private sh: ShellSettings = {
    prefer: "auto",
    effective: GIT_BASH,
    auto: GIT_BASH,
    options: [GIT_BASH, PWSH, POWERSHELL],
    platform: "windows",
  };

  async shell(): Promise<ShellSettings> {
    return { ...this.sh };
  }

  // A path nobody has is the failure the pane has to render, so it is refused
  // here the way the kernel refuses it rather than quietly accepted.
  async saveShell(prefer: string, path: string): Promise<ShellSettings> {
    const found = (this.sh.options ?? []).find((o) => (path ? o.path === path : o.prefer === prefer));
    if (path && !found) throw new Error(`这个 shell 用不了：${path}: no such executable`);
    this.sh = { ...this.sh, prefer, path, effective: prefer === "auto" ? this.sh.auto : (found ?? this.sh.auto) };
    return { ...this.sh };
  }
}
