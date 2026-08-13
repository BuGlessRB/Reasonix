import type { AgentPort, ApprovalMode, ApprovalVerdict, HistoryMessage, ModelEntry, Preset, ProviderSetup, SessionEntry, SessionStatus } from "./port";
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
  { wait: 500, ev: { kind: "tool_dispatch", tool: tool("read_file", { args: "internal/provider/retry.go" }) } },
  { wait: 700, ev: { kind: "tool_result", tool: tool("read_file", { output: "203 行", durationMs: 940 }) } },
  { wait: 100, ev: usage(12400, 0, 120) },
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
        output: "A 组（无退避）  401 × 7  200 × 193\nB 组（200ms 退避后重试）  401 × 0  200 × 200",
        durationMs: 64000,
      }),
    },
  },
  { wait: 100, ev: usage(6100, 400, 180, "subagent") },
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

  async sessions(): Promise<SessionEntry[]> {
    return [{ name: "default", path: "/sessions/default.jsonl", current: true }];
  }

  async newSession() {
    this.log = [];
    this.at = 0;
    this.state.goal = "";
  }

  async deleteSession(_name: string) {}

  async status() {
    return { ...this.state };
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
    this.state.goal = text;
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
