import type { AccountState, AgentPort, Completion, CompletionItem, DeviceGrant, ProviderCheck, ProviderEdit, ProviderEntry, ProviderProbe, VersionHub, ApprovalMode, ApprovalVerdict, Checkpoint, RewindPlan, RewindResult, RewindScope, HistoryMessage, ModelEntry, Preset, ProviderSetup, RoleAssignments, SessionEntry, SessionStatus, McpDraft, McpDraftServer, McpEntry, McpInstallResult, HookCatalog, HookDryRun, HookEntry, MemoryCatalog, MemoryEntry, NetworkProbe, NetworkSettings, McpRisk, SkillCatalog, SkillEntry, WorkspaceInfo, WorkspaceChanges, Attachment, ThemePack } from "./port";
import type { WireEvent } from "./wire";
import { SCRIPT } from "./fixture";


export class MockPort implements AgentPort {
  private listeners = new Set<(ev: WireEvent) => void>();
  private log: WireEvent[] = [];
  // What the user has sent, so checkpoints() can mirror one per turn.
  private prompts: string[] = [];
  private undone: string[] | null = null;
  private at = 0;
  private timer: number | undefined;
  // The script pauses on approval_request/ask_request the same way the real
  // run blocks on Approve()/AnswerQuestion(); nothing advances until answered.
  private gated = false;
  private state: SessionStatus = {
    label: "deepseek-v4-pro",
    running: false,
    plan: false,
    preset: "balanced",
    effort: "auto",
    modelRef: "deepseek/deepseek-v4-pro",
    toolApprovalMode: "ask",
    autoApproveTools: false,
    bypass: false,
    goal: "",
    goalStatus: "stopped",
    cwd: "~/projects/DeepSeek-Reasonix/.reasonix/sessions",
    workspaceRoot: "~/projects/DeepSeek-Reasonix",
    used: 0,
    window: 128000,
    cacheHit: 0,
    cacheMiss: 0,
  };

  private setupDone = false;
  private session: SessionEntry | null = null;

  async providerSetup(): Promise<ProviderSetup | null> {
    return this.setupDone ? null : { required: true, provider: "deepseek", model: "deepseek-v4-pro", keyEnv: "DEEPSEEK_API_KEY" };
  }

  async saveProviderKey(_apiKey: string) {
    this.setupDone = true;
  }

  // The subagent runs somewhere cheaper; everything else rides the main model.
  private assigned: RoleAssignments = {
    planner: "",
    subagent: "deepseek/deepseek-v4-flash",
    guardian: "",
    vision: "",
  };

  async roles(): Promise<RoleAssignments> {
    return this.assigned;
  }

  // The fixture's endpoints answer both protocols, which is the case the row's
  // "改用…" repair exists for.
  async checkProvider(name: string): Promise<ProviderCheck> {
    if (name === "mimo") return { ok: false, error: "401 unauthorized: key 过期了" };
    return { ok: true, kind: "openai", models: ["gpt-4o", "claude-sonnet-4"], ambiguous: true };
  }


  async setRole(role: string, ref: string) {
    this.assigned = { ...this.assigned, [role]: ref };
  }

  // Two protocols onto one host, plus a second vendor carrying the only model
  // that reads images: the two shapes the picker has to render correctly.
  async models(): Promise<ModelEntry[]> {
    const efforts = ["auto", "low", "high", "max"];
    return [
      {
        ref: "deepseek/deepseek-v4-pro", provider: "deepseek", model: "deepseek-v4-pro",
        kind: "openai", vendor: "api.deepseek.com", keyEnv: "DEEPSEEK_API_KEY", active: true, efforts, effort: "high",
        contextWindow: 131072, price: { input: 2, output: 8, currency: "CNY" },
      },
      {
        ref: "deepseek-anthropic/deepseek-v4-pro", provider: "deepseek-anthropic",
        model: "deepseek-v4-pro", kind: "anthropic", vendor: "api.deepseek.com", keyEnv: "DEEPSEEK_API_KEY",
        efforts, effort: "high", contextWindow: 131072,
      },
      {
        ref: "deepseek/deepseek-v4-flash", provider: "deepseek", model: "deepseek-v4-flash",
        kind: "openai", vendor: "api.deepseek.com", keyEnv: "DEEPSEEK_API_KEY", efforts, effort: "high",
        contextWindow: 131072, price: { input: 0.5, output: 2, currency: "CNY" },
      },
      {
        ref: "kimi/kimi-k2-vision", provider: "kimi", model: "kimi-k2-vision",
        kind: "openai", vendor: "api.moonshot.cn", keyEnv: "KIMI_API_KEY", vision: true, contextWindow: 262144,
      },
      {
        ref: "myrelay/gpt-4o", provider: "myrelay", model: "gpt-4o", kind: "openai",
        vendor: "relay.example.com", keyEnv: "MYRELAY_API_KEY", vision: true, contextWindow: 131072,
      },
      {
        ref: "myrelay/claude-sonnet-4", provider: "myrelay", model: "claude-sonnet-4", kind: "openai",
        vendor: "relay.example.com", keyEnv: "MYRELAY_API_KEY", contextWindow: 200000,
      },
      {
        ref: "myrelay-work/gpt-4o", provider: "myrelay-work", model: "gpt-4o", kind: "openai",
        vendor: "relay.example.com", keyEnv: "MYRELAY_WORK_API_KEY", contextWindow: 131072,
      },
    ];
  }

  // Mutable: the admin switches are the whole point of the extensions page, and
  // a fixture that answers the same list either way cannot show them working.
  private activeTheme = "";

  private servers: McpEntry[] = [
    {
      name: "time",
      state: "ready",
      enabled: true,
      transport: "stdio",
      source: "built-in",
      tools: 2,
      toolNames: ["get_current_time", "convert_time"],
    },
    { name: "context7", state: "idle", enabled: true, tools: 0, transport: "http" },
    { name: "figma", state: "failed", enabled: true, transport: "http", tools: 0, error: "401 unauthorized" },
  ];

  private skillList: SkillEntry[] = [
    {
      name: "review", slashName: "review", description: "复核这一轮改动，给出严重度分级",
      scope: "project", path: ".reasonix/skills/review/SKILL.md", subagent: true, enabled: true,
    },
    { name: "init", slashName: "init", description: "为这个仓库生成一份项目说明", scope: "builtin", enabled: true },
    {
      name: "security-review", slashName: "security-review", description: "只读地过一遍安全面",
      scope: "builtin", subagent: true, readOnly: true, effort: "high", enabled: true,
    },
    // No slash name of its own: only model discovery reaches it. This is the
    // row the old list could not show at all.
    {
      name: "release-notes", description: "从提交历史起草发布说明",
      scope: "global", path: "~/.reasonix/skills/release-notes/SKILL.md", enabled: false,
    },
    {
      name: "deploy-runbook", slashName: "deploy-runbook", description: "只在你点名时跑的部署清单",
      scope: "project", path: ".reasonix/skills/deploy-runbook/SKILL.md", manual: true, enabled: true,
    },
  ];

  async mcp(): Promise<McpEntry[]> {
    return this.servers.map((s) => ({ ...s }));
  }

  async reconnectMcp(name: string) {
    const s = this.servers.find((x) => x.name === name);
    if (!s) return { state: "failed", error: "no configured MCP server named " + name };
    // figma keeps failing: a retry that always works would hide the state the
    // button exists for.
    if (name === "figma") {
      s.state = "failed";
      s.error = "401 unauthorized";
      return { state: "failed", error: s.error };
    }
    s.state = "ready";
    s.error = undefined;
    s.tools = s.tools || 1;
    return { state: "ready", tools: s.tools };
  }

  async setMcpEnabled(name: string, enabled: boolean) {
    const s = this.servers.find((x) => x.name === name);
    if (!s) return;
    s.enabled = enabled;
    s.state = enabled ? (s.error ? "failed" : "idle") : "disabled";
  }

  // A rough stand-in for internal/mcpsetup: enough shape for the confirmation
  // card to be developed against, not a second implementation of the grammar.
  async parseMcp(input: string): Promise<McpDraft> {
    const text = input.trim();
    if (!text) throw new Error("粘贴一段 JSON、一行命令，或者一个 https 地址");
    if (text.startsWith("{")) {
      const doc = JSON.parse(text) as { mcpServers?: Record<string, Record<string, unknown>> };
      const servers = doc.mcpServers ?? (doc as Record<string, Record<string, unknown>>);
      const out: McpDraftServer[] = Object.entries(servers).map(([name, spec]) => ({
        name,
        transport: typeof spec.url === "string" ? "http" : "stdio",
        command: spec.command as string | undefined,
        args: spec.args as string[] | undefined,
        url: spec.url as string | undefined,
        env: spec.env as Record<string, string> | undefined,
      }));
      if (out.length === 0) throw new Error("这段 JSON 里没有 MCP 服务");
      return { servers: out, risks: risksOf(out) };
    }
    if (/^https?:\/\//.test(text)) {
      const host = new URL(text).hostname.replace(/^www\./, "").split(".")[0];
      const server: McpDraftServer = { name: host, transport: "http", url: text };
      return { servers: [server], risks: risksOf([server]) };
    }
    const argv = text.replace(/^[$>%]\s+/, "").split(/\s+/);
    const pkg = argv.slice(1).find((a) => !a.startsWith("-")) ?? argv[0];
    const server: McpDraftServer = {
      name: pkg.split("/").pop()!.split("@")[0] || "mcp-server",
      transport: "stdio",
      command: argv[0],
      args: argv.slice(1),
    };
    return { servers: [server], risks: risksOf([server]) };
  }

  async installMcp(server: McpDraftServer, _scope: "user" | "project"): Promise<McpInstallResult> {
    if (this.servers.some((s) => s.name === server.name)) {
      return { name: server.name, state: "issue", toolCount: 0, action: "retry", message: `已经有一个叫 ${server.name} 的服务了` };
    }
    // A name with "auth" in it stands in for the OAuth path, so the
    // action_required branch is reachable in dev.
    if (/auth|figma/i.test(server.name)) {
      this.servers.push({ ...toEntry(server), state: "failed", error: "401 unauthorized" });
      return { name: server.name, state: "action_required", toolCount: 0, action: "authenticate", message: "需要先授权" };
    }
    this.servers.push({ ...toEntry(server), state: "ready", tools: 3, toolNames: ["one", "two", "three"] });
    return { name: server.name, state: "ready", toolCount: 3, action: "none", message: "" };
  }

  async removeMcp(name: string) {
    const before = this.servers.length;
    this.servers = this.servers.filter((s) => s.name !== name);
    return { disconnected: before !== this.servers.length, stillConfigured: false };
  }

  private hookList: HookEntry[] = [
    { event: "PostToolUse", match: "edit_file|write_file", command: "gofmt -w .", scope: "global", usesMatch: true },
    { event: "PreToolUse", match: "bash", command: "./scripts/guard.sh", scope: "global", blocking: true, usesMatch: true },
  ];

  async hooks(): Promise<HookCatalog> {
    return {
      hooks: this.hookList.map((h) => ({ ...h })),
      sources: [{ scope: "global", path: "~/.reasonix/settings.json", status: "ok", hookCount: this.hookList.length }],
      events: [
        { name: "PreToolUse", blocking: true, usesMatch: true },
        { name: "PostToolUse", blocking: false, usesMatch: true },
        { name: "PostToolUseFailure", blocking: false, usesMatch: true },
        { name: "PermissionRequest", blocking: false, usesMatch: true },
        { name: "UserPromptSubmit", blocking: true, usesMatch: false },
        { name: "Stop", blocking: false, usesMatch: false },
        { name: "SessionStart", blocking: false, usesMatch: false },
        { name: "Notification", blocking: false, usesMatch: false },
      ],
      projectPath: "~/projects/DeepSeek-Reasonix/.reasonix/settings.json",
      globalPath: "~/.reasonix/settings.json",
    };
  }

  async saveHooks(scope: "user" | "project", hooks: HookEntry[]) {
    this.hookList = hooks.map((h) => ({ ...h, scope: scope === "user" ? "global" : "project" }));
  }

  // Stands in for a real execution: a command mentioning "guard" exits 2 so the
  // blocking verdict is reachable in dev.
  async dryRunHook(h: HookEntry): Promise<HookDryRun> {
    const bad = /guard|exit 2/.test(h.command);
    return {
      decision: bad ? "block" : "pass",
      exitCode: bad ? 2 : 0,
      stdout: bad ? "" : "ok",
      stderr: bad ? "refusing: .env is protected" : "",
      durationMs: 120,
      blocks: bad && !!h.blocking,
    };
  }

  private net: NetworkSettings = {
    mode: "auto",
    effective: "环境变量 HTTPS_PROXY=http://127.0.0.1:7890",
    direct: ["api.mimo.cn"],
    endpoint: "https://api.deepseek.com/v1",
  };

  private mem: MemoryEntry[] = [
    {
      name: "no-coauthored-by", title: "提交信息不带 Co-Authored-By",
      description: "也不要 Generated with 之类的署名脚注", activation: "pinned",
      scope: "project", type: "feedback", updatedAt: "2026-06-11",
      body: "提交信息里不要出现 Co-Authored-By，PR 描述里也不要生成署名脚注。",
      path: "~/.reasonix/projects/reasonix/memory/no-coauthored-by.md",
    },
    {
      name: "reply-language", title: "回复用中文",
      description: "跟着用户每条消息的语言走", activation: "pinned",
      scope: "global", type: "user", updatedAt: "2026-05-02",
      body: "回复语言跟随用户当前这条消息的语言。",
    },
    {
      name: "v2-rewrite", title: "v2 是从零重写的 Go 内核",
      description: "没有 web，桌面端重做；main-v2 是默认分支", activation: "relevant",
      scope: "project", type: "project", updatedAt: "2026-05-30",
      body: "v2 = 从零重写的 Go 内核。不带 web；桌面端重做。main-v2 是默认分支。",
      usedLastTurn: true, why: "问题里提到了 main-v2 和分支",
    },
    {
      name: "old-build-flag", title: "构建用 -tags legacy",
      description: "迁移前的构建方式", activation: "relevant",
      scope: "project", type: "reference", updatedAt: "2025-11-02", expired: true,
      body: "构建时加 -tags legacy。",
    },
  ];

  async memories(): Promise<MemoryCatalog> {
    return {
      memories: this.mem.map((m) => ({ ...m })),
      recallQuery: "缓存命中率为什么会掉",
      indexPath: "~/.reasonix/projects/reasonix/memory/MEMORY.md",
    };
  }

  async forgetMemory(name: string) {
    this.mem = this.mem.filter((m) => m.name !== name);
  }

  async network(): Promise<NetworkSettings> {
    return { ...this.net };
  }

  async saveNetwork(s: NetworkSettings, password: string, clearPassword: boolean) {
    this.net = {
      ...s,
      hasPassword: clearPassword ? false : s.hasPassword || !!password,
      effective:
        s.mode === "off"
          ? "不走代理（已关闭）"
          : s.mode === "custom"
            ? `${s.type || "http"}://${s.username ? s.username + ":••••@" : ""}${s.server || "?"}:${s.port || 0}`
            : "环境变量 HTTPS_PROXY=http://127.0.0.1:7890",
    };
    return { ...this.net };
  }

  // Stands in for a real walk: a custom proxy pointing at a port nothing is
  // listening on is the common misconfiguration, so it fails at connect.
  async diagnoseNetwork(): Promise<NetworkProbe[]> {
    const custom = this.net.mode === "custom";
    const out: NetworkProbe[] = [
      { step: "proxy", ok: true, detail: this.net.effective, durationMs: 0 },
      { step: "dns", ok: true, detail: "proxy.corp → 10.0.0.9", durationMs: 21 },
    ];
    if (custom && (this.net.port ?? 0) === 0) {
      out.push({
        step: "connect",
        ok: false,
        detail: "连不上 proxy.corp:0：connection refused",
        durationMs: 12,
        advice: "解析得到地址但连不上代理，检查代理是不是没开、端口对不对",
      });
      return out;
    }
    out.push({ step: "connect", ok: true, detail: "通了", durationMs: 180 });
    out.push({ step: "tls", ok: true, detail: "握手成功 · HTTP 200", durationMs: 240 });
    out.push({
      step: "auth",
      ok: false,
      detail: "key 被拒了 — HTTP 401",
      durationMs: 310,
      advice: "网络通了，是 key 的问题，去「模型」那页换一个",
    });
    return out;
  }

  async skills(): Promise<SkillCatalog> {
    return { implicit: true, skills: this.skillList.map((s) => ({ ...s })) };
  }

  async setSkillEnabled(name: string, enabled: boolean) {
    const sk = this.skillList.find((x) => x.name === name);
    if (sk) sk.enabled = enabled;
  }

  async versions(): Promise<VersionHub> {
    return { current: "dev", pinned: "", stalePin: false, latest: "", newer: false, versions: [] };
  }

  // Three shapes the account grouping has to keep apart: one vendor reached
  // under two protocols, a custom relay serving other vendors' models, and two
  // tenants of that same relay holding different keys.
  private sources: ProviderEntry[] = [
    {
      name: "deepseek", kind: "openai", baseUrl: "https://api.deepseek.com",
      models: ["deepseek-v4-pro", "deepseek-v4-flash"], default: "deepseek-v4-pro",
      hasKey: true, inUse: true, preset: false, keyEnv: "DEEPSEEK_API_KEY", canSetVision: false,
    },
    {
      name: "deepseek-anthropic", kind: "anthropic", baseUrl: "https://api.deepseek.com/anthropic",
      models: ["deepseek-v4-pro"], default: "deepseek-v4-pro",
      hasKey: true, inUse: false, preset: false, keyEnv: "DEEPSEEK_API_KEY",
      canSetVision: false, canWebSearch: true, webSearch: true,
    },
    {
      name: "myrelay", kind: "openai", baseUrl: "https://relay.example.com/v1",
      models: ["gpt-4o", "claude-sonnet-4"], default: "gpt-4o",
      hasKey: true, inUse: false, preset: false, keyEnv: "MYRELAY_API_KEY",
      visionModels: ["gpt-4o"], canSetVision: true,
    },
    {
      name: "myrelay-work", kind: "openai", baseUrl: "https://relay.example.com/v1",
      models: ["gpt-4o"], default: "gpt-4o",
      hasKey: true, inUse: false, preset: false, keyEnv: "MYRELAY_WORK_API_KEY",
    },
  ];

  async providers(): Promise<ProviderEntry[]> {
    return this.sources;
  }

  async probeProvider(): Promise<ProviderProbe> {
    throw new Error("演示模式不会真的去连端点");
  }

  async saveProvider(): Promise<void> {}

  async setProviderWebSearch(name: string, on: boolean): Promise<void> {
    this.sources = this.sources.map((p) => (p.name === name ? { ...p, webSearch: on } : p));
  }

  async editProvider(edit: ProviderEdit): Promise<void> {
    this.sources = this.sources.map((p) =>
      p.name === edit.name ? { ...p, models: edit.models, default: edit.default, visionModels: edit.vision } : p,
    );
  }

  async removeProvider(): Promise<void> {}

  async pinVersion(): Promise<void> {}

  async goToVersion(): Promise<void> {
    throw new Error("演示模式不会真的安装版本");
  }

  onUpdateProgress(): () => void {
    return () => {};
  }

  async account(): Promise<AccountState> {
    return { signedIn: false };
  }

  async accountLogin(): Promise<DeviceGrant> {
    throw new Error("没有内核，无法登录");
  }

  async accountPoll(): Promise<{ status: "pending" | "complete" }> {
    return { status: "pending" };
  }

  async accountLogout(): Promise<void> {}

  async workspaces(): Promise<WorkspaceInfo> {
    return {
      current: this.state.workspaceRoot ?? "",
      canSwitch: true,
      canIsolate: true,
      recents: [
        { path: "~/projects/reasonix-site", name: "reasonix-site" },
        { path: "~/work/notes", name: "notes" },
      ],
    };
  }

  async setWorkspace(path: string) {
    this.state.workspaceRoot = path;
    this.state.cwd = path;
    await this.newSession();
  }

  async isolateWorkspace() {
    await this.setWorkspace((this.state.workspaceRoot ?? "") + " (隔离副本)");
  }

  async pickFolder() {
    return "";
  }

  // A demo shell has no workspace to read and no kernel to ask, so the fixture
  // answers from a tree of its own. Deliberately the short version of the
  // grammar: enough to drive the menu, never the place to look up what "@" or
  // "/" mean — that answer lives in control.Complete.
  private tree = [
    "REASONIX.md",
    "go.mod",
    "internal/control/complete.go",
    "internal/control/controller.go",
    "internal/serve/serve.go",
    "desktop/frontend-next/src/ui/Composer.tsx",
    "desktop/frontend-next/src/ui/Completion.tsx",
  ];

  private builtins: CompletionItem[] = [
    { label: "/compact", insert: "/compact ", hint: "压缩上下文，保留结论", kind: "builtin" },
    { label: "/context", insert: "/context", hint: "看这一会话的上下文占用", kind: "builtin" },
    { label: "/clear", insert: "/clear", hint: "清空上下文，留在同一会话", kind: "builtin" },
    { label: "/rewind", insert: "/rewind", hint: "回到某一轮之前", kind: "builtin" },
    { label: "/model", insert: "/model ", hint: "换模型", descend: true, kind: "builtin" },
    { label: "/memory", insert: "/memory ", hint: "看和管这个项目记住的事", descend: true, kind: "builtin" },
  ];

  private commands: CompletionItem[] = [
    { label: "/commit", insert: "/commit ", hint: "把当前改动整理成一条提交", kind: "command" },
    { label: "/review", insert: "/review ", hint: "复核这一轮改动，给出严重度分级", kind: "subagent" },
    { label: "/init", insert: "/init ", hint: "为这个仓库生成一份项目说明", kind: "skill" },
    { label: "/security-review", insert: "/security-review ", hint: "只读地过一遍安全面", kind: "subagent" },
  ];

  async complete(line: string, cursor: number): Promise<Completion> {
    const before = line.slice(0, cursor);
    const at = before.lastIndexOf("@");
    if (at >= 0 && !/\s/.test(before.slice(at + 1)) && (at === 0 || /\s/.test(line[at - 1]))) {
      const rest = line.slice(at + 1).search(/\s/);
      const to = rest < 0 ? line.length : at + 1 + rest;
      const frag = before.slice(at + 1);
      const dir = frag.includes("/") ? frag.slice(0, frag.lastIndexOf("/") + 1) : "";
      const names = new Set<string>();
      for (const path of this.tree) {
        if (!path.startsWith(dir)) continue;
        const rel = path.slice(dir.length);
        const cut = rel.indexOf("/");
        names.add(cut < 0 ? rel : rel.slice(0, cut + 1));
      }
      const items = [...names]
        .filter((n) => n.startsWith(frag.slice(dir.length)))
        .sort((a, b) => Number(b.endsWith("/")) - Number(a.endsWith("/")))
        .map((n) => ({
          label: n,
          insert: "@" + dir + n,
          descend: n.endsWith("/"),
          kind: n.endsWith("/") ? "dir" : "file",
        }));
      return { kind: "ref", from: at, to, query: frag.slice(dir.length), items };
    }
    if (line.startsWith("/") && !/\s/.test(line)) {
      const q = line.toLowerCase();
      const items = [...this.builtins, ...this.commands].filter((it) => it.label.toLowerCase().startsWith(q));
      return { kind: "slash", from: 0, to: line.length, query: line, items };
    }
    return { kind: "", from: 0, to: 0, items: [] };
  }

  // Matches the kernel: the shell mints no session file at launch, so the list
  // is empty until a turn creates one. A static entry here is why a rail that
  // never refetched looked fine in dev and was blank in the real app.
  async sessions(): Promise<SessionEntry[]> {
    if (!this.session) return [];
    return [{ ...this.session, current: true }];
  }

  async newSession() {
    this.log = [];
    this.at = 0;
    this.state.goal = "";
    this.session = null;
    this.state.sessionPath = undefined;
  }

  async deleteSession(_name: string) {}

  async status() {
    return { ...this.state };
  }

  // The fixture has no working tree behind it, so the panel falls back to what
  // the transcript says rather than claiming git reported nothing changed.
  // No host behind the fixture, so a paste resolves to a token nothing reads.
  async attach(): Promise<Attachment> {
    return { path: ".reasonix/attachments/mock.png", ref: "@.reasonix/attachments/mock.png" };
  }

  async changes(): Promise<WorkspaceChanges> {
    return { repo: false, changes: [] };
  }

  async trajectory(): Promise<WireEvent[]> {
    return [];
  }

  async history(): Promise<HistoryMessage[]> {
    return [];
  }

  // Mock mode has to be able to show the rewind entry, so every prompt it has
  // seen becomes a checkpoint the way the kernel opens one per user turn.
  async checkpoints(): Promise<Checkpoint[]> {
    return this.prompts.map((prompt, i) => ({ turn: i, prompt, files: i === 0 ? 0 : 3 }));
  }

  // The second prompt onwards is scripted to have run bash, so mock mode can
  // show the consent stage the real kernel demands on partial coverage.
  async prepareRewind(turn: number, scope: RewindScope): Promise<RewindPlan> {
    const partial = turn > 0;
    return {
      planId: `mock-plan-${turn}-${scope}`,
      turn,
      coverage: partial ? "partial" : "full",
      coverageGaps: partial
        ? [{ reason: "bash_side_effect", detail: "bash side effects are not path-tracked", tool: "bash" }]
        : undefined,
      canFiles: true,
      canConversation: true,
      files: ["note.txt"],
      fileCount: turn > 0 ? 3 : 0,
      requiresConfirmation: partial,
    };
  }

  async commitRewind(planId: string): Promise<RewindResult> {
    const turn = Number(planId.split("-")[2] ?? 0);
    this.undone = this.prompts.slice();
    this.prompts = this.prompts.slice(0, turn);
    return { ok: true, transactionId: `mock-tx-${turn}`, undoAvailable: true, deleted: ["note.txt"] };
  }

  async undoRewind(_transactionId: string): Promise<void> {
    if (this.undone) this.prompts = this.undone;
  }

  subscribe(onEvent: (ev: WireEvent) => void) {
    this.listeners.add(onEvent);
    return () => this.listeners.delete(onEvent);
  }

  private emit(ev: WireEvent) {
    this.log.push(ev);
    this.listeners.forEach((l) => l(ev));
  }

  private step = () => {
    if (this.gated || !this.state.running) return;
    const beat = SCRIPT[this.at];
    if (!beat) {
      this.state.running = false;
      return;
    }
    this.at += 1;
    this.emit(beat.ev);
    if (beat.ev.kind === "turn_done") {
      this.state.running = false;
      return;
    }
    if (beat.ev.kind === "approval_request" || beat.ev.kind === "ask_request") {
      this.gated = true;
      return;
    }
    const next = SCRIPT[this.at];
    if (next) this.timer = window.setTimeout(this.step, next.wait);
  };

  private ungate() {
    if (!this.gated) return;
    this.gated = false;
    this.state.running = true;
    const next = SCRIPT[this.at];
    if (next) this.timer = window.setTimeout(this.step, next.wait);
  }

  async steer(text: string) {
    this.emit({ kind: "steer", text });
  }

  async submit(text: string) {
    if (this.state.running) {
      this.emit({ kind: "steer", text });
      return;
    }
    this.prompts.push(text);
    // The first turn is what puts the session on disk. serve answers with a
    // truncated first message straight away and swaps in the generated title
    // once the background job lands, so the rail is never left blank.
    if (!this.session) {
      const name = "20260813-004512.881204300-deepseek-v4-pro";
      this.session = { name, path: `/sessions/${name}.jsonl`, title: text.slice(0, 47), turns: 0 };
      this.state.sessionPath = this.session.path;
      setTimeout(() => this.session && (this.session.title = "定位仓库里失败的测试"), 2500);
    }
    this.session.turns = (this.session.turns ?? 0) + 1;
    // Not the goal: the kernel only sets that from /goal or plan mode. Echoing
    // the prompt into it made the header read the same text twice.
    this.state.running = true;
    this.at = 0;
    this.emit({ kind: "message", text, itemId: "user" });
    this.timer = window.setTimeout(this.step, SCRIPT[0].wait);
  }

  async cancel() {
    window.clearTimeout(this.timer);
    this.state.running = false;
  }

  async resume(_path: string) {
    this.log = [];
    this.at = 0;
  }

  async approve(_id: string, verdict: ApprovalVerdict) {
    if (verdict === "deny") {
      this.state.running = false;
      this.gated = false;
      return;
    }
    if (verdict === "always") this.state.toolApprovalMode = "dontAsk";
    this.ungate();
  }

  async answer(_id: string, _answers: { questionId: string; selected: string[] }[]) {
    this.ungate();
  }

  // Two packs so the picker has something to switch between; the fixture is
  // where the mapping gets exercised without a Go process.
  async themes(): Promise<ThemePack[]> {
    return [
      {
        id: "dusk", name: "Dusk", author: "fixture", active: this.activeTheme === "dusk",
        tokens: {
          light: { bg: "#F6F3EE", bgSoft: "#FBF9F5", panel: "#FFFFFF", border: "#DDD5C8", fg: "#1B1814", fgDim: "#5F564A", accent: "#8A5A2B" },
          dark: { bg: "#0F0D0B", bgSoft: "#141110", panel: "#1B1715", border: "#332C26", fg: "#EFE9E1", fgDim: "#9A8F82", accent: "#D89B5A" },
        },
      },
      {
        id: "tide", name: "Tide", author: "fixture", active: this.activeTheme === "tide",
        tokens: {
          light: { bg: "#F2F6F8", bgSoft: "#F9FCFD", panel: "#FFFFFF", border: "#CBD9E0", fg: "#12191C", fgDim: "#4E5D65", accent: "#0E6E82" },
          dark: { bg: "#080D10", bgSoft: "#0C1316", panel: "#131C21", border: "#23333A", fg: "#E4EEF2", fgDim: "#8298A2", accent: "#4FB6CE" },
        },
      },
    ];
  }
  async activateTheme(id: string) {
    this.activeTheme = id;
  }

  // No sidecar runs behind the fixture, so an invocation answers the way a
  // connected extension would rather than pretending to have done work.
  async invokeExtensionAction(name: string) {
    return `${name} 在 mock 里没有真实扩展可执行`;
  }
  async submitExtensionForm(_pluginId: string, _surfaceId: string, _values: Record<string, unknown>) {}

  async setPlanMode(on: boolean) {
    this.state.plan = on;
  }
  async setApprovalMode(mode: ApprovalMode) {
    this.state.toolApprovalMode = mode;
  }
  async setPreset(preset: Preset) {
    this.state.preset = preset;
  }
  async setModel(ref: string) {
    this.state.modelRef = ref;
    this.state.label = ref.split("/").pop() ?? ref;
  }
  async setEffort(effort: string) {
    this.state.effort = effort;
  }
  async setGoal(text: string) {
    this.state.goal = text;
  }
}

function toEntry(s: McpDraftServer): McpEntry {
  return {
    name: s.name,
    state: "idle",
    enabled: true,
    transport: s.transport,
    source: "user config",
    tools: 0,
  };
}

const SECRETY = /auth|token|secret|credential|api[-_]?key|cookie/i;

function risksOf(servers: McpDraftServer[]): McpRisk[] {
  const out: McpRisk[] = [];
  for (const s of servers) {
    if (s.command) {
      out.push({ server: s.name, kind: "shell", field: "command", detail: [s.command, ...(s.args ?? [])].join(" ") });
    }
    if (s.url) out.push({ server: s.name, kind: "unknown-host", field: "url", detail: s.url });
    for (const [k, v] of Object.entries(s.env ?? {})) {
      if (SECRETY.test(k) && !v.startsWith("$")) {
        out.push({ server: s.name, kind: "secret", field: "env." + k,
          detail: `明文写进配置文件；改成 \${${k.toUpperCase()}} 可以只留在环境变量里` });
      }
    }
  }
  return out;
}

