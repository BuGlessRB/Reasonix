import type { AgentPort, ApprovalMode, ApprovalVerdict, HistoryMessage, ModelEntry, Preset, ProviderSetup, SessionEntry, SessionStatus, McpDraft, McpDraftServer, McpEntry, McpInstallResult, HookCatalog, HookDryRun, HookEntry, NetworkProbe, NetworkSettings, McpRisk, SkillCatalog, SkillEntry, SlashEntry, WorkspaceInfo } from "./port";
import type { WireEvent } from "./wire";

interface Beat {
  wait: number;
  ev: WireEvent;
}

const tool = (name: string, over: Partial<WireEvent["tool"] & object> = {}): WireEvent["tool"] => ({
  id: name + "-" + Math.floor(performance.now()),
  name,
  readOnly: true,
  ...over,
});

const usage = (hit: number, miss: number, out: number, source = "executor"): WireEvent => ({
  kind: "usage",
  usage: {
    promptTokens: hit + miss,
    completionTokens: out,
    totalTokens: hit + miss + out,
    cacheHitTokens: hit,
    cacheMissTokens: miss,
    sessionCacheHitTokens: hit,
    sessionCacheMissTokens: miss,
    source,
    currency: "¥",
  },
});

const SCRIPT: Beat[] = [
  { wait: 300, ev: { kind: "turn_started" } },
  {
    wait: 900,
    ev: {
      kind: "reasoning",
      text: "401 有两种可能：key 真的失效，或者网关瞬时态。两种处置完全相反，先别改代码。",
    },
  },
  {
    wait: 700,
    ev: {
      kind: "text",
      text: "先翻历史 —— #3146 和 #4106 都记着这是网关瞬时态，我们从不删 key。",
    },
  },
  { wait: 100, ev: usage(2100, 180, 40) },
  { wait: 500, ev: { kind: "tool_dispatch", tool: tool("todo_write") } },
  {
    wait: 600,
    ev: {
      kind: "tool_result",
      tool: tool("todo_write", {
        output: JSON.stringify([
          "翻 #3146 / #4106 的历史结论",
          "定位 provider 侧重试路径",
          "用真实 curl 做 A/B 复现",
          "确认是否网关瞬时态",
          "给修法，不动 key 存储",
        ]),
      }),
    },
  },
  // Three reads in a row: the spec collapses a run into one manifest, so the
  // fixture has to produce a run. Outputs carry read_file's real line numbering.
  ...[
    ["internal/provider/retry.go", 203],
    ["internal/config/credentials.go", 88],
    ["internal/agent/agent.go", 412],
  ].flatMap(([path, n], i): Beat[] => [
    {
      wait: 400,
      ev: { kind: "tool_dispatch", tool: tool("read_file", { id: `rd${i}`, args: JSON.stringify({ path }) }) },
    },
    {
      wait: 500,
      ev: {
        kind: "tool_result",
        tool: tool("read_file", {
          id: `rd${i}`,
          args: JSON.stringify({ path }),
          output: Array.from({ length: Number(n) }, (_, k) => `${String(k + 1).padStart(3)}→// ${path}:${k + 1}`).join("\n"),
          durationMs: 210 + i * 40,
        }),
      },
    },
  ]),
  { wait: 100, ev: usage(12400, 0, 120) },
  // ls: a directory is "name/" with no size, a file is "name<TAB>bytes".
  {
    wait: 500,
    ev: {
      kind: "tool_result",
      tool: tool("ls", {
        id: "ls1",
        args: JSON.stringify({ path: "internal/provider" }),
        output: ["openai/", "anthropic/", "retry.go\t8420", "retry_test.go\t11304", "provider.go\t5177"].join("\n"),
        durationMs: 60,
      }),
    },
  },
  // grep: "path:line:text" exactly as internal/tool/builtin/grep.go formats it,
  // trailing truncation note included.
  {
    wait: 500,
    ev: {
      kind: "tool_result",
      tool: tool("grep", {
        id: "gr1",
        args: JSON.stringify({ pattern: "forget\\(provider\\)" }),
        output: [
          "internal/config/credentials.go:205:\ts.forget(provider)",
          "internal/provider/retry.go:88:\t\t\tstore.forget(provider)",
          "internal/agent/agent.go:1841:\t// 瞬时 401 不该走 forget(provider)",
          "... (truncated at 3 matches)",
        ].join("\n"),
        durationMs: 140,
      }),
    },
  },
  // An external service answering, and a named skill running as a subagent. Both
  // render differently from a built-in call — the server badge and the nested
  // trace — and neither had a fixture, so neither was ever seen in dev.
  {
    wait: 450,
    ev: {
      kind: "tool_result",
      tool: tool("use_capability", {
        id: "mcp1",
        resolvedName: "mcp__time__get_current_time",
        args: JSON.stringify({ timezone: "Asia/Shanghai" }),
        output: "2026-08-13T14:22:07+08:00",
        durationMs: 120,
      }),
    },
  },
  { wait: 400, ev: { kind: "tool_dispatch", tool: tool("task", { id: "tk1", profile: { name: "security-review" } }) } },
  {
    wait: 500,
    ev: {
      kind: "tool_result",
      tool: tool("grep", {
        id: "tk1-a", parentId: "tk1",
        args: JSON.stringify({ pattern: "api_key" }),
        output: "internal/config/credentials.go:41:\tKey string `toml:\"api_key\"`",
        durationMs: 90,
      }),
    },
  },
  {
    wait: 600,
    ev: {
      kind: "tool_result",
      tool: tool("task", {
        id: "tk1",
        profile: { name: "security-review" },
        args: JSON.stringify({ description: "只读地过一遍安全面" }),
        output: "没有新增的密钥读写路径；退避补丁不触碰凭据存储。",
        durationMs: 8400,
      }),
    },
  },
  {
    wait: 400,
    ev: { kind: "notice", level: "warn", text: "工作树里有未提交的改动，退避补丁会叠在上面。" },
  },
  {
    wait: 600,
    ev: {
      kind: "guardian_assessment",
      guardian: {
        id: "g1",
        tool: "bash",
        subject: "curl A/B · 200 次并发",
        outcome: "放行",
        risk_level: "medium",
        rationale: "对外发压是可观测的副作用，不是纯读。范围收在单 key、单分钟、无写操作。",
      },
    },
  },
  {
    wait: 800,
    ev: {
      kind: "tool_result",
      tool: tool("bash", {
        readOnly: false,
        args: "curl A/B · 200 次并发 · 同一个 key",
        output: [
          "$ for i in $(seq 200); do curl -s -o /dev/null -w '%{http_code}\\n' \"$EP\"; done | sort | uniq -c",
          "",
          "A 组（无退避）",
          "    7 401",
          "  193 200",
          "",
          "B 组（200ms 退避后重试）",
          "    0 401",
          "  200 200",
          "",
          "同一个 key，同一分钟。401 只落在 A 组。",
        ].join("\n"),
        durationMs: 64000,
        execution: { kind: "shell", shell: "git-bash", platform: "windows", state: "completed", exitCode: 0 },
      }),
    },
  },
  { wait: 100, ev: usage(6100, 400, 180, "subagent") },
  { wait: 400, ev: { kind: "compaction_started", compaction: { trigger: "auto" } } },
  {
    wait: 900,
    ev: {
      kind: "compaction_done",
      compaction: {
        trigger: "auto",
        messages: 34,
        summary: "401 已证实是网关瞬时态（curl A/B，A 组 7 次 B 组 0 次）。处置是退避重试，不删 key。",
        archive: "~/.reasonix/projects/F--Reasonix/archive/2026-08-13.jsonl",
      },
    },
  },
  {
    wait: 600,
    ev: {
      kind: "approval_request",
      approval: {
        id: "a1",
        tool: "edit_file",
        subject: "internal/config/credentials.go · 第 205–210 行",
        reason: "这一步会写你的工作树。",
        kind: "tool",
      },
    },
  },
  {
    wait: 700,
    ev: {
      kind: "tool_result",
      tool: tool("edit_file", {
        readOnly: false,
        args: "internal/config/credentials.go",
        added: 6,
        removed: 4,
        diff: "@@ -205,6 +205,8 @@\n-\ts.forget(provider)\n+\treturn s.retryAfter(200 * time.Millisecond)",
      }),
    },
  },
  {
    wait: 600,
    ev: {
      kind: "ask_request",
      ask: {
        id: "q1",
        questions: [
          {
            header: "退避范围",
            id: "q1-1",
            prompt: "curl A/B 证实是网关瞬时态。要不要一并给 provider 层加通用的 401 退避？",
            options: [
              { label: "一并改 retry.go", description: "根因是一个，影响所有 provider。" },
              { label: "只保留 credentials.go 这处", description: "范围守住不外溢。" },
            ],
          },
        ],
      },
    },
  },
  {
    wait: 800,
    ev: { kind: "text", text: "两处都改了，测试全绿。没动 key 存储的任何一行 —— 那条纪律保住了。" },
  },
  { wait: 100, ev: usage(700, 90, 240) },
  {
    wait: 400,
    ev: {
      kind: "completion_summary",
      completion: {
        preset: "delivery",
        verdict: "complete",
        mutations: 2,
        checks_passed: 3,
        checks_failed: 0,
        checks_suppressed: 1,
        review: "passed",
        gap_kinds: ["suppressed"],
        constraint_degraded: false,
      },
    },
  },
  { wait: 300, ev: { kind: "turn_done" } },
];

export class MockPort implements AgentPort {
  private listeners = new Set<(ev: WireEvent) => void>();
  private log: WireEvent[] = [];
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

  async models(): Promise<ModelEntry[]> {
    return [
      { ref: "deepseek/deepseek-v4-pro", provider: "deepseek", model: "deepseek-v4-pro", active: true },
      { ref: "deepseek/deepseek-v4-flash", provider: "deepseek", model: "deepseek-v4-flash" },
    ];
  }

  // Mutable: the admin switches are the whole point of the extensions page, and
  // a fixture that answers the same list either way cannot show them working.
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

  async slash(): Promise<SlashEntry[]> {
    return [
      { name: "commit", kind: "command", description: "把当前改动整理成一条提交", argHint: "[范围]" },
      { name: "review", kind: "skill", description: "复核这一轮改动，给出严重度分级", scope: "project", subagent: true },
      { name: "init", kind: "skill", description: "为这个仓库生成一份项目说明", scope: "builtin" },
      { name: "security-review", kind: "skill", description: "只读地过一遍安全面", scope: "builtin", subagent: true },
    ];
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

  async trajectory(): Promise<WireEvent[]> {
    return [];
  }

  async history(): Promise<HistoryMessage[]> {
    return [];
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
