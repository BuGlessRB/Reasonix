import type { WireEvent } from "./wire";

// The scripted turn MockPort replays. It lives apart from the port because it is
// content, not transport: every card the UI can draw needs a beat here, or that
// rendering path is never seen during development.
export interface Beat {
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

export const SCRIPT: Beat[] = [
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
  // The agent teaching itself something, which no other tool call does — and the
  // card for it had no fixture, so it was never seen in dev.
  {
    wait: 500,
    ev: {
      kind: "tool_result",
      tool: tool("remember", {
        id: "rm1",
        readOnly: false,
        args: JSON.stringify({
          name: "gateway-401-transient",
          title: "网关的 401 是瞬时态",
          description: "退避重试即可，不要删 key",
          scope: "project",
          activation: "relevant",
          body: "curl A/B 证实：同一个 key 同一分钟，401 只落在无退避的一组。",
        }),
        output: "Saved memory id=mem_01 revision=1 (project background) as gateway-401-transient",
        durationMs: 40,
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
        // A fold that dropped one of its own changes: the case the card exists
        // for, since the summary above reads just as finished either way.
        sourceTokens: 128_400,
        projectionTokens: 31_200,
        coverageRequired: 12,
        coverageMissing: 2,
        coverageRepaired: true,
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
