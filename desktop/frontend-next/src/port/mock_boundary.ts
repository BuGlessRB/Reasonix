import type { PermissionLists, PermissionRules, SandboxSettings } from "./port";
import { MockShell } from "./mock_shell";

// The boundary half of the fixture: what the agent is refused outright, and how
// far an approved write reaches. It carries one rule of each kind already, so
// the pane renders the three lists populated rather than as three empty boxes —
// an empty editor cannot show what a rule looks like.
export class MockBoundary extends MockShell {
  private rules: PermissionRules = {
    mode: "ask",
    deny: ["bash(git push:*)", "file_mutation(*.env*)"],
    ask: ["bash(rm:*)"],
    allow: ["bash(go test:*)"],
    path: "/Users/you/.reasonix/config.toml",
  };

  private jail: SandboxSettings = {
    bash: "enforce",
    network: true,
    workspaceRoot: "",
    allowWrite: ["/tmp/scratch"],
    effectiveWriteRoots: ["/Users/you/code/site", "/tmp/scratch"],
    available: true,
    platform: "darwin",
    path: "/Users/you/.reasonix/config.toml",
  };

  async permissions(): Promise<PermissionRules> {
    return { ...this.rules };
  }

  // A rule the gate's own parser would reject is refused here the same way, so
  // the pane's error path is reachable from the fixture.
  async savePermissions(lists: PermissionLists): Promise<PermissionRules> {
    for (const rule of [...lists.deny, ...lists.ask, ...lists.allow]) {
      if (!/^[^()]+(\(.*\))?$/.test(rule.trim())) throw new Error(`invalid permission rule "${rule}"`);
    }
    this.rules = { ...this.rules, ...lists };
    return { ...this.rules };
  }

  async sandbox(): Promise<SandboxSettings> {
    return { ...this.jail };
  }

  async saveSandbox(s: SandboxSettings): Promise<SandboxSettings> {
    const roots = [s.workspaceRoot || "/Users/you/code/site", ...s.allowWrite.filter(Boolean)];
    this.jail = { ...this.jail, ...s, effectiveWriteRoots: roots };
    return { ...this.jail };
  }
}
