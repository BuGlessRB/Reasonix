import type { Ask, Approval, Compaction, Guardian, Tool, WireEvent } from "../port/wire";
import type { HistoryMessage } from "../port/port";

export type Item =
  | { t: "user"; id: string; text: string; pending?: boolean }
  | { t: "say"; id: string; text: string; reasoning?: string; done: boolean }
  | { t: "tool"; id: string; tool: Tool; running: boolean; children: Tool[] }
  | { t: "guardian"; id: string; g: Guardian }
  | { t: "approval"; id: string; a: Approval; verdict?: string }
  | { t: "ask"; id: string; ask: Ask; answered?: string[][] }
  | { t: "compaction"; id: string; c: Compaction; done: boolean }
  | { t: "notice"; id: string; level: string; text: string };

export interface Metrics {
  hit: number;
  miss: number;
  out: number;
  bySource: Record<string, number>;
  cost: number;
  currency: string;
}

export interface Waiting {
  ttftSince?: number;
  retry?: { attempt: number; max: number };
}

export interface PlanStep {
  text: string;
  done: boolean;
}

export interface SessionState {
  error: string;
  items: Item[];
  plan: PlanStep[];
  metrics: Metrics;
  waiting: Waiting;
  running: boolean;
  doing: string;
  steerQueue: string[];
}

export const initialState: SessionState = {
  error: "",
  items: [],
  plan: [],
  metrics: { hit: 0, miss: 0, out: 0, bySource: {}, cost: 0, currency: "¥" },
  waiting: {},
  running: false,
  doing: "空闲",
  steerQueue: [],
};

let seq = 0;
const nextId = () => `i${++seq}`;

// tool_dispatch and tool_result are two phases of one call; the UI shows one
// row that flips from running to settled, so they fold onto the same item.
function foldTool(items: Item[], tool: Tool, running: boolean): Item[] {
  // A subagent's calls carry parentId; they belong inside the task that spawned
  // them, not as siblings in the main flow.
  if (tool.parentId) {
    const at = items.findIndex((i) => i.t === "tool" && i.tool.id === tool.parentId);
    if (at >= 0) {
      const parent = items[at] as Extract<Item, { t: "tool" }>;
      const kids = parent.children.slice();
      const k = kids.findIndex((c) => c.id === tool.id);
      if (k >= 0) kids[k] = { ...kids[k], ...tool };
      else kids.push(tool);
      const next = items.slice();
      next[at] = { ...parent, children: kids };
      return next;
    }
  }
  const key = tool.id;
  if (key) {
    const at = items.findIndex((i) => i.t === "tool" && i.tool.id === key);
    if (at >= 0) {
      const prev = items[at] as Extract<Item, { t: "tool" }>;
      const next = items.slice();
      next[at] = { ...prev, tool: { ...prev.tool, ...tool }, running };
      return next;
    }
  }
  return [...items, { t: "tool", id: nextId(), tool, running, children: [] }];
}

function appendText(items: Item[], text: string, field: "text" | "reasoning"): Item[] {
  const last = items[items.length - 1];
  if (last && last.t === "say" && !last.done) {
    const next = items.slice();
    next[next.length - 1] = { ...last, [field]: (last[field] ?? "") + text };
    return next;
  }
  return [...items, { t: "say", id: nextId(), text: "", done: false, [field]: text }];
}

export function reduce(
  s: SessionState,
  ev:
    | WireEvent
    | { kind: "__restore"; items: Item[]; plan: PlanStep[]; hit: number; miss: number; cost?: number }
    | { kind: "__error"; text: string },
): SessionState {
  if (ev.kind === "__error") return { ...s, error: ev.text };
  if (ev.kind === "__restore") {
    return {
      ...s,
      items: ev.items,
      plan: ev.plan,
      metrics: { ...s.metrics, hit: ev.hit, miss: ev.miss, cost: ev.cost ?? s.metrics.cost },
    };
  }
  switch (ev.kind) {
    case "turn_started":
      return { ...s, running: true, doing: "运行中", waiting: { ttftSince: Date.now() } };

    case "reasoning":
      return { ...s, doing: "思考中", waiting: {}, items: appendText(s.items, ev.text ?? "", "reasoning") };

    case "text":
      return { ...s, doing: "正在回答", waiting: {}, items: appendText(s.items, ev.text ?? "", "text") };

    case "message": {
      if (ev.itemId === "user") {
        return { ...s, items: [...s.items, { t: "user", id: nextId(), text: ev.text ?? "" }] };
      }
      const items = s.items.slice();
      for (let i = items.length - 1; i >= 0; i--) {
        if (items[i].t === "say") {
          items[i] = { ...(items[i] as Extract<Item, { t: "say" }>), done: true };
          break;
        }
      }
      return { ...s, items };
    }

    case "tool_dispatch":
      return ev.tool
        ? { ...s, doing: ev.tool.name, items: foldTool(s.items, ev.tool, true) }
        : s;

    case "tool_progress":
      return ev.tool ? { ...s, items: foldTool(s.items, ev.tool, true) } : s;

    case "tool_result": {
      if (!ev.tool) return s;
      const plan = (ev.tool.name === "todo_write" && parsePlan(ev.tool)) || s.plan;
      return { ...s, plan, items: foldTool(s.items, ev.tool, false) };
    }

    case "usage": {
      const u = ev.usage;
      if (!u) return s;
      const src = u.source || "executor";
      const spent = quoteAmount(u.costQuote) ?? u.cost ?? 0;
      return {
        ...s,
        metrics: {
          hit: s.metrics.hit + u.cacheHitTokens,
          miss: s.metrics.miss + u.cacheMissTokens,
          out: s.metrics.out + u.completionTokens,
          bySource: { ...s.metrics.bySource, [src]: (s.metrics.bySource[src] ?? 0) + spent },
          cost: s.metrics.cost + spent,
          currency: u.costQuote?.original.currency || u.currency || s.metrics.currency,
        },
      };
    }

    case "guardian_assessment":
      return ev.guardian
        ? { ...s, items: [...s.items, { t: "guardian", id: nextId(), g: ev.guardian }] }
        : s;

    case "approval_request":
      return ev.approval
        ? { ...s, doing: "等你批准", items: [...s.items, { t: "approval", id: nextId(), a: ev.approval }] }
        : s;

    case "ask_request":
      return ev.ask
        ? { ...s, doing: "等你决定", items: [...s.items, { t: "ask", id: nextId(), ask: ev.ask }] }
        : s;

    case "compaction_started":
      return { ...s, items: [...s.items, { t: "compaction", id: nextId(), c: ev.compaction ?? {}, done: false }] };

    case "compaction_done": {
      const items = s.items.slice();
      for (let i = items.length - 1; i >= 0; i--) {
        if (items[i].t === "compaction") {
          items[i] = { t: "compaction", id: items[i].id, c: ev.compaction ?? {}, done: true };
          break;
        }
      }
      return { ...s, items };
    }

    case "retrying":
      return {
        ...s,
        waiting: { ...s.waiting, retry: { attempt: ev.retryAttempt ?? 0, max: ev.retryMax ?? 0 } },
      };

    case "steer": {
      const q = s.steerQueue.filter((t) => t !== ev.text);
      const items = s.items.map((i) =>
        i.t === "user" && i.pending && i.text === ev.text ? { ...i, pending: false } : i,
      );
      return { ...s, steerQueue: q, items };
    }

    case "notice":
      return {
        ...s,
        items: [...s.items, { t: "notice", id: nextId(), level: ev.level ?? "info", text: ev.text ?? "" }],
      };

    case "turn_done":
      return { ...s, running: false, doing: ev.err ? "失败" : "已完成", waiting: {} };

    default:
      return s;
  }
}

// The kernel wraps each user turn in control-plane blocks (language policy,
// execution policy). They are instructions to the model, not something the
// user said, so they never reach the transcript.
const CONTROL = /<(reasoning-language|response-language|execution-policy)[\s\S]*?<\/\1>\s*/g;
const stripControl = (s: string) => s.replace(CONTROL, "").trim();

// The quote is the authoritative amount: it carries currency and an estimated
// flag, while usage.cost is only a legacy alias.
export function quoteAmount(q?: { selected?: { amount: string }; original: { amount: string } }): number | undefined {
  const raw = q?.selected?.amount ?? q?.original.amount;
  if (raw === undefined) return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : undefined;
}

// todo_write carries the plan as its payload; the panel needs it as state, not
// as one more line that scrolls away.
// The todos live in args; output is only a receipt ("Todos updated: 3 total").
// Returning null keeps the existing plan instead of overwriting it with junk.
function parsePlan(tool: Tool): PlanStep[] | null {
  try {
    const v = JSON.parse(tool.args ?? "");
    const list = Array.isArray(v?.todos) ? v.todos : Array.isArray(v) ? v : null;
    if (!list) return null;
    return list.map((x: { content?: string; status?: string }) => ({
      text: String(x.content ?? ""),
      done: x.status === "completed",
    }));
  } catch {
    return null;
  }
}

// A reload has no event stream to replay, so the transcript is rebuilt from the
// provider conversation. Control-plane turns (system, and the language preamble
// the kernel prepends to each user message) are not part of what was said.
export function fromHistory(msgs: HistoryMessage[]): { items: Item[]; plan: PlanStep[] } {
  const out: Item[] = [];
  let plan: PlanStep[] = [];
  for (const m of msgs) {
    if (m.role === "system") continue;
    if (m.role === "user") {
      const text = stripControl(m.content);
      if (text) out.push({ t: "user", id: nextId(), text });
      continue;
    }
    if (m.role === "assistant") {
      if (m.content || m.reasoning) {
        out.push({ t: "say", id: nextId(), text: m.content, reasoning: m.reasoning, done: true });
      }
      for (const c of m.toolCalls ?? []) {
        if (c.name === "todo_write") {
          const p = parsePlan({ name: c.name, args: c.arguments, readOnly: true });
          if (p) plan = p;
        }
        out.push({
          t: "tool",
          id: nextId(),
          tool: { id: c.id, name: c.name, args: c.arguments, readOnly: true },
          running: false,
          children: [],
        });
      }
      continue;
    }
    if (m.role === "tool") {
      const at = out.findIndex((i) => i.t === "tool" && i.tool.id === m.toolCallId);
      if (at >= 0) {
        const prev = out[at] as Extract<Item, { t: "tool" }>;
        out[at] = { ...prev, tool: { ...prev.tool, output: m.content } };
      }
    }
  }
  return { items: out, plan };
}
