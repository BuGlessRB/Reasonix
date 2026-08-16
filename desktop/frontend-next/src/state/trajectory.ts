import type { WireEvent } from "../port/wire";

export type Span = { t: string } | { b: string } | { n: string };

export interface TrajRow {
  seq: number;
  at: number;
  kind: string;
  payload: Span[];
  subs: Span[][];
  // 时间轴画的是事实，不是格式化过的字：一条记录代表的活动跑了多久（秒），
  // 以及是哪个工具跑的 —— 颜色由界面那层按类别决定。
  dur?: number;
  tool?: string;
}

export interface TrajState {
  rows: TrajRow[];
  t0: number;
}

export const initialTraj: TrajState = { rows: [], t0: 0 };

const kb = (n: number) => (n / 1000).toFixed(1) + "k";

function record(ev: WireEvent): { kind: string; payload: Span[]; subs?: Span[][]; dur?: number; tool?: string } | null {
  switch (ev.kind) {
    case "turn_started":
      return { kind: "turn_started", payload: [{ t: "run begin" }] };

    case "message":
      return ev.itemId === "user"
        ? { kind: "event", payload: [{ t: "user_message" }] }
        : { kind: "event", payload: [{ t: "assistant_text" }] };

    case "tool_dispatch":
    case "tool_result": {
      const tool = ev.tool;
      if (!tool) return null;
      // A partial dispatch is one call streaming its arguments in, not a second
      // call; recording it doubles every row.
      if (ev.kind === "tool_dispatch" && tool.partial) return null;
      const head: Span[] = [
        { t: ev.kind === "tool_dispatch" ? "tool_call " : "tool_result " },
        { b: tool.resolvedName || tool.name },
      ];
      if (tool.parentId) head.push({ t: " ↳ " }, { b: tool.parentId });
      if (tool.durationMs != null) head.push({ t: " · " }, { n: (tool.durationMs / 1000).toFixed(2) + "s" });
      if (tool.err) head.push({ t: " · err=" }, { b: tool.err });
      return {
        kind: "event",
        payload: head,
        tool: tool.resolvedName || tool.name,
        dur: tool.durationMs != null ? tool.durationMs / 1000 : undefined,
      };
    }

    case "usage": {
      const u = ev.usage;
      if (!u) return null;
      return {
        kind: "event",
        payload: [
          { t: "usage · hit " },
          { n: kb(u.cacheHitTokens) },
          { t: " · miss " },
          { n: kb(u.cacheMissTokens) },
          { t: " · out " },
          { n: kb(u.completionTokens) },
          { t: " · src=" },
          { b: u.source || "executor" },
        ],
      };
    }

    case "guardian_assessment": {
      const g = ev.guardian;
      if (!g) return null;
      return {
        kind: "readiness_audit",
        payload: [{ t: "tool=" }, { b: g.tool }, { t: " · verdict=" }, { b: g.outcome }],
        subs: g.rationale ? [[{ t: g.rationale }]] : [],
        tool: g.tool,
      };
    }

    case "approval_request":
      return ev.approval
        ? {
            kind: "delegation_admission",
            payload: [{ t: "approval tool=" }, { b: ev.approval.tool }, { t: " · " + ev.approval.subject }],
            tool: ev.approval.tool,
          }
        : null;

    case "ask_request":
      return ev.ask
        ? { kind: "event", payload: [{ t: "ask_request · " }, { n: String(ev.ask.questions.length) }, { t: " 个问题" }] }
        : null;

    case "compaction_started":
      return { kind: "outcome_progress", payload: [{ t: "compaction started" }] };

    case "compaction_done":
      return {
        kind: "outcome_progress",
        payload: [{ t: "compaction done · " }, { n: String(ev.compaction?.messages ?? 0) }, { t: " 条" }],
      };

    case "retrying":
      return {
        kind: "protocol_recovery",
        payload: [
          { t: "retry " },
          { n: `${ev.retryAttempt ?? 0}/${ev.retryMax ?? 0}` },
          { t: " · scope=" },
          { b: ev.retryScope ?? "stream" },
        ],
      };

    case "steer":
      return { kind: "memory_recall", payload: [{ t: "steer delivered · " }, { b: ev.text ?? "" }] };

    case "context_maintenance":
      return { kind: "outcome_progress", payload: [{ t: ev.text || "context maintenance" }] };

    case "completion_summary": {
      const c = ev.completion;
      if (!c) return null;
      return {
        kind: "completion_report",
        payload: [
          { t: "verdict=" },
          { b: c.verdict },
          { t: " · mutations " },
          { n: String(c.mutations) },
          { t: " · checks " },
          { n: `${c.checks_passed}/${c.checks_passed + c.checks_failed}` },
        ],
        subs: c.review ? [[{ t: c.review }]] : [],
      };
    }

    case "notice":
      return {
        kind: ev.code ? "protocol_recovery" : "event",
        payload: [{ t: "notice " }, { b: ev.level ?? "info" }, { t: " · " + (ev.text ?? "") }],
      };

    case "turn_done":
      return { kind: "event", payload: [{ t: ev.err ? "turn_done · err=" + ev.err : "turn_done" }] };

    default:
      return null;
  }
}

export function reduceTraj(
  s: TrajState,
  ev: WireEvent | { kind: "__clear" } | { kind: "__user"; text: string },
): TrajState {
  if (ev.kind === "__clear") return initialTraj;
  // The kernel emits no event for your own turn, so the row that opens every
  // trajectory has to be written here or it is missing entirely.
  const r =
    ev.kind === "__user"
      ? { kind: "event", payload: [{ t: "user_message · " }, { b: ev.text.slice(0, 60) }] as Span[] }
      : record(ev as WireEvent);
  if (!r) return s;
  const now = Date.now();
  const t0 = s.t0 || now;
  return {
    t0,
    rows: [
      ...s.rows,
      {
        seq: s.rows.length + 1,
        at: (now - t0) / 1000,
        kind: r.kind,
        payload: r.payload,
        subs: r.subs ?? [],
        dur: r.dur,
        tool: r.tool,
      },
    ],
  };
}
