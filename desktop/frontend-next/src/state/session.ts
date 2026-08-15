import type { Ask, Approval, Compaction, CompletionSummary, ExtensionSurface, Guardian, Tool, WireEvent } from "../port/wire";
import { estimateTokens, sample, type Sample } from "../port/tokens";
import type { HistoryMessage } from "../port/port";

export type Item =
  | { t: "user"; id: string; text: string; pending?: boolean }
  | { t: "say"; id: string; text: string; reasoning?: string; done: boolean; thoughtMs?: number }
  | { t: "tool"; id: string; tool: Tool; running: boolean; children: Tool[] }
  | { t: "reads"; id: string; tools: Tool[] }
  | { t: "guardian"; id: string; g: Guardian }
  | { t: "approval"; id: string; a: Approval; verdict?: string }
  | { t: "ask"; id: string; ask: Ask; answered?: string[][] }
  | { t: "compaction"; id: string; c: Compaction; done: boolean }
  | { t: "remember"; id: string; m: RememberedFact; forgotten?: boolean }
  | { t: "completion"; id: string; c: CompletionSummary }
  | { t: "extension"; id: string; ext: ExtensionSurface }
  | { t: "notice"; id: string; level: string; text: string };

// What a remember call wrote, read off its own arguments. Saving a fact changes
// what the agent will do in later sessions, which no other tool call does — so it
// gets a card of its own rather than scrolling past as one more step.
export interface RememberedFact {
  name: string;
  title: string;
  description: string;
  scope: string;
  activation: string;
  body: string;
}

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
  // Recent streamed-output samples. The down-rate has to come from what arrived
  // in the last few seconds; usage totals only land at round boundaries, and
  // nothing at all arrives while a tool runs.
  outWindow: Sample[];
  metrics: Metrics;
  waiting: Waiting;
  running: boolean;
  doing: string;
  steerQueue: string[];
  // Standing extension surfaces, keyed by plugin and surface id. They describe
  // a state that is still true, so they hold a place in the side rail instead
  // of scrolling away in the transcript.
  panels: ExtensionSurface[];
  // Composed views. A view is a standing surface by definition — it describes
  // something that is still true — so it never joins the transcript, and where
  // it is drawn is decided at render time rather than here. That is what lets
  // the user move one without any of this having to be re-sorted.
  views: ExtensionSurface[];
  // Views that replace a card the host would have drawn, keyed by anchor. They
  // are kept apart from `views` because they have no place of their own: they
  // appear only where the thing they stand in for appears.
  takeovers: Record<string, ExtensionSurface>;
}

export const initialState: SessionState = {
  error: "",
  items: [],
  plan: [],
  outWindow: [],
  metrics: { hit: 0, miss: 0, out: 0, bySource: {}, cost: 0, currency: "¥" },
  waiting: {},
  running: false,
  doing: "空闲",
  steerQueue: [],
  panels: [],
  views: [],
  takeovers: {},
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

// dropTool removes the dispatch row a specialised card replaces, so the same
// call is not shown twice once its result arrives.
function dropTool(items: Item[], id?: string): Item[] {
  if (!id) return items;
  return items.filter((i) => !(i.t === "tool" && i.tool.id === id));
}

// The card reads the call's own arguments: they carry what was saved and how it
// will be recalled, which the tool's textual receipt does not.
function parseRemembered(tool: Tool): RememberedFact | null {
  try {
    const a = JSON.parse(tool.args ?? "{}") as Record<string, string>;
    const name = (a.name || "").trim();
    const description = (a.description || "").trim();
    if (!name && !description) return null;
    return {
      name,
      title: (a.title || "").trim() || name,
      description,
      scope: (a.scope || "project").trim(),
      activation: (a.activation || "relevant").trim(),
      body: (a.body || "").trim(),
    };
  } catch {
    return null;
  }
}

// Settles the message still being written. turn_done settles every open one:
// a tool call between two answers leaves the earlier card unsealed forever, and
// an unsealed card keeps a caret blinking and its reveal clock running on text
// nothing will add to. Returning the same array keeps every card's memo intact.
function sealSay(items: Item[], all = false): Item[] {
  let next: Item[] | null = null;
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (it.t !== "say" || it.done) continue;
    next ??= items.slice();
    const ran = it.thoughtMs ?? (it.reasoning ? Date.now() - (thoughtSince.get(it.id) ?? Date.now()) : undefined);
    thoughtSince.delete(it.id);
    next[i] = { ...it, done: true, thoughtMs: ran };
    if (!all) break;
  }
  return next ?? items;
}

// read_file is the most-called tool by a wide margin — 96 calls across a sample
// of recent sessions, against 47 for bash. One card each is the noise the spec
// collapses into a single manifest. Merging happens here rather than at render
// time so each card keeps a stable identity and stays memoised.
// The spec's manifest is one step for a whole run of lookups, not for reads
// alone: its own fixture folds grep and glob rows in beside the files. A group
// still has to be anchored by a read — a lone grep is better served by its own
// excerpt list than by a row that says only how many times it matched.
const LOOKUP = new Set(["read_file", "grep", "glob", "ls"]);

const lookup = (i: Item | undefined) =>
  i?.t === "tool" && !i.running && LOOKUP.has(i.tool.name) && i.children.length === 0;

const toolOf = (i: Item) => (i as Extract<Item, { t: "tool" }>).tool;

function mergeReads(items: Item[]): Item[] {
  const n = items.length;
  const last = items[n - 1];
  if (!lookup(last)) return items;
  const tool = toolOf(last);
  const prev = items[n - 2];
  if (prev?.t === "reads") {
    const next = items.slice(0, n - 1);
    next[n - 2] = { ...prev, tools: [...prev.tools, tool] };
    return next;
  }
  if (lookup(prev) && (tool.name === "read_file" || toolOf(prev).name === "read_file")) {
    const next = items.slice(0, n - 1);
    next[n - 2] = { t: "reads", id: prev.id, tools: [toolOf(prev), tool] };
    return next;
  }
  return items;
}

function appendText(items: Item[], text: string, field: "text" | "reasoning"): Item[] {
  const last = items[items.length - 1];
  if (last && last.t === "say" && !last.done) {
    const next = items.slice();
    // Thinking runs until the first answer token, so the clock is the gap
    // between the two streams, not the length of either.
    const stop = field === "text" && last.thoughtMs === undefined && last.reasoning;
    next[next.length - 1] = {
      ...last,
      [field]: (last[field] ?? "") + text,
      ...(stop ? { thoughtMs: Date.now() - (thoughtSince.get(last.id) ?? Date.now()) } : null),
    };
    return next;
  }
  const id = nextId();
  if (field === "reasoning") thoughtSince.set(id, Date.now());
  return [...items, { t: "say", id, text: "", done: false, [field]: text }];
}

// Keyed by item id rather than carried on the item: a start time is not part of
// what the card renders, and putting it there would make every append rewrite it.
const thoughtSince = new Map<string, number>();

export function reduce(
  s: SessionState,
  ev:
    | WireEvent
    | { kind: "__restore"; items: Item[]; plan: PlanStep[]; hit: number; miss: number; cost?: number }
    | { kind: "__error"; text: string }
    | { kind: "__user"; text: string; pending: boolean }
    | { kind: "__decided"; id: string; verdict?: string; answers?: string[][] }
    | { kind: "__forgot"; id: string },
): SessionState {
  if (ev.kind === "__error") return { ...s, error: ev.text };
  // Both event.Message emitters carry assistant text, so nothing on the wire
  // echoes what you typed — only /history has it, and only after a reload. The
  // client owns its own turn. Mid-turn input stays pending until the steer
  // event says the run consumed it at a tool boundary.
  if (ev.kind === "__user") {
    return {
      ...s,
      steerQueue: ev.pending ? [...s.steerQueue, ev.text] : s.steerQueue,
      items: [...s.items, { t: "user", id: nextId(), text: ev.text, pending: ev.pending }],
    };
  }
  // The card stays after the fact is dropped, marked: the transcript is the
  // record of what happened, and erasing the row would erase that too.
  if (ev.kind === "__forgot") {
    return {
      ...s,
      items: s.items.map((i) => (i.t === "remember" && i.id === ev.id ? { ...i, forgotten: true } : i)),
    };
  }

  // The card reads its sealed state off the item, so the decision has to be
  // recorded here — otherwise an answered question stays answerable forever.
  if (ev.kind === "__decided") {
    // Answering hands the turn back to the tool, which may then run for a
    // minute. Leaving the label on 等你批准 says the opposite of what is
    // happening, and it is the one line the user watches to know anything is.
    const decided = s.items.find((i) => i.id === ev.id);
    const resumed =
      decided?.t === "approval" && ev.verdict !== "deny" ? decided.a.tool || "运行中" : s.doing;
    return {
      ...s,
      doing: decided?.t === "ask" ? "运行中" : resumed,
      items: s.items.map((i) =>
        i.id !== ev.id
          ? i
          : i.t === "approval"
            ? { ...i, verdict: ev.verdict ?? "once" }
            : i.t === "ask"
              ? { ...i, answered: ev.answers ?? [] }
              : i,
      ),
    };
  }
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
      return {
        ...s,
        doing: "思考中",
        waiting: {},
        outWindow: sample(s.outWindow, estimateTokens(ev.text ?? ""), Date.now()),
        items: appendText(s.items, ev.text ?? "", "reasoning"),
      };

    case "text":
      return {
        ...s,
        doing: "正在回答",
        waiting: {},
        outWindow: sample(s.outWindow, estimateTokens(ev.text ?? ""), Date.now()),
        items: appendText(s.items, ev.text ?? "", "text"),
      };

    case "message":
      return { ...s, items: sealSay(s.items) };

    case "tool_dispatch":
      return ev.tool
        ? { ...s, doing: ev.tool.name, items: foldTool(s.items, ev.tool, true) }
        : s;

    case "tool_progress":
      return ev.tool ? { ...s, items: foldTool(s.items, ev.tool, true) } : s;

    case "tool_result": {
      if (!ev.tool) return s;
      // A failed remember is an ordinary failed tool call; only a save that
      // actually landed is worth its own card.
      const fact = ev.tool.name === "remember" && !ev.tool.err ? parseRemembered(ev.tool) : null;
      if (fact) {
        return { ...s, items: [...dropTool(s.items, ev.tool.id), { t: "remember", id: nextId(), m: fact }] };
      }
      // A delegate keeps its own todo list, and it is not the plan the user is
      // watching. Without the parentId guard the rail flips to the subagent's
      // steps mid-turn: the user's own completed items lose their strike and
      // line one turns into somebody else's first step.
      const own = ev.tool.name === "todo_write" && !ev.tool.parentId;
      const plan = (own && parsePlan(ev.tool)) || s.plan;
      return { ...s, plan, items: mergeReads(foldTool(s.items, ev.tool, false)) };
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

    // A surface is addressed by id, so a re-publish is an update: a status the
    // extension refreshes while it works would otherwise pile up one card per
    // tick instead of moving in place.
    case "extension_surface":
    case "extension_status": {
      const ext = ev.extension;
      if (!ext) return s;
      if (ext.kind === "panel") {
        const at = s.panels.findIndex((p) => p.pluginId === ext.pluginId && p.surfaceId === ext.surfaceId);
        if (at < 0) return { ...s, panels: [...s.panels, ext] };
        const panels = s.panels.slice();
        panels[at] = ext;
        return { ...s, panels };
      }
      if (ext.kind === "view" && ext.view?.anchor) {
        return { ...s, takeovers: { ...s.takeovers, [ext.view.anchor]: ext } };
      }
      if (ext.kind === "view") {
        const at = s.views.findIndex((v) => v.pluginId === ext.pluginId && v.surfaceId === ext.surfaceId);
        if (at < 0) return { ...s, views: [...s.views, ext] };
        const views = s.views.slice();
        views[at] = ext;
        return { ...s, views };
      }
      const at = s.items.findIndex(
        (it) => it.t === "extension" && it.ext.pluginId === ext.pluginId && it.ext.surfaceId === ext.surfaceId,
      );
      if (at < 0) return { ...s, items: [...s.items, { t: "extension", id: nextId(), ext }] };
      const items = s.items.slice();
      items[at] = { t: "extension", id: items[at].id, ext };
      return { ...s, items };
    }

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

    // The digest streams in while the fold runs. It accumulates on the card's
    // own summary so the finished event simply replaces it — a fold that dies
    // mid-write leaves what it had written rather than an empty placeholder.
    case "compaction_progress": {
      if (!ev.text) return s;
      const items = s.items.slice();
      for (let i = items.length - 1; i >= 0; i--) {
        const it = items[i];
        if (it.t === "compaction" && !it.done) {
          items[i] = { ...it, c: { ...it.c, summary: (it.c.summary ?? "") + ev.text } };
          break;
        }
      }
      return { ...s, items };
    }

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

    // The kernel already suppresses the boring case: a summary only reaches the
    // wire when the turn mutated state or ended anything but cleanly.
    case "completion_summary":
      return ev.completion
        ? { ...s, items: [...s.items, { t: "completion", id: nextId(), c: ev.completion }] }
        : s;

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
      // A readiness complaint ends the turn without the work having failed, and
      // the reason is the only part worth reading. Anything but a clean finish
      // keeps the run amber rather than claiming the tick.
      // Sealing here too: a turn that ends without a closing message otherwise
      // leaves the caret blinking on a message nothing will be appended to.
      // A plan that ran to the end is spent: it says nothing the completion card
      // does not, and leaving it struck through in the rail reads as if the next
      // turn already has a plan. One that still has open steps stays — that is
      // the half the user needs.
      return {
        ...s,
        running: false,
        doing: ev.err ? ev.err : "已完成",
        waiting: {},
        plan: s.plan.length > 0 && s.plan.every((p) => p.done) ? [] : s.plan,
        items: sealSay(s.items, true),
      };

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
export function parsePlan(tool: Tool): PlanStep[] | null {
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

// The listing the kernel writes for the model: "- **title**" and, indented under
// it, "<url>". Only a run of exactly that gets lifted out of the prose — reading
// a paragraph as a result list is worse than leaving the list in place.
const SEARCH_TITLE = /^-\s+\*\*.+\*\*\s*$/;
const SEARCH_URL = /^\s+<\S+>\s*$/;

function splitProviderSearch(content: string): { text: string; search?: boolean }[] {
  const lines = content.split("\n");
  const blocks: [number, number][] = [];
  for (let i = 0; i < lines.length; ) {
    if (!SEARCH_TITLE.test(lines[i])) {
      i++;
      continue;
    }
    let end = i;
    let urls = 0;
    while (end < lines.length && (SEARCH_TITLE.test(lines[end]) || SEARCH_URL.test(lines[end]))) {
      if (SEARCH_URL.test(lines[end])) urls++;
      end++;
    }
    // A run with no source is the model's own bolded list — an answer written as
    // "- **厄瓜多尔总统将访华**" reads exactly like a result title.
    if (urls > 0) blocks.push([i, end]);
    i = end > i ? end : i + 1;
  }
  const parts: { text: string; search?: boolean }[] = [];
  const push = (from: number, to: number, search: boolean) => {
    const text = lines.slice(from, to).join("\n").trim();
    if (text) parts.push(search ? { text, search: true } : { text });
  };
  let at = 0;
  for (const [from, to] of blocks) {
    push(at, from, false);
    push(from, to, true);
    at = to;
  }
  push(at, lines.length, false);
  return parts;
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
        // A provider-run search leaves its listing inside the assistant text —
        // that copy is the only one the next turn has — while the live stream
        // shows it as a card. Cut it back out here, or reopening the same turn
        // replaces the card with forty lines of prose.
        let reasoning = m.reasoning;
        const parts = splitProviderSearch(m.content);
        // Thinking came before the search that follows it, so it cannot ride the
        // next text part when the turn opened with a search.
        if (reasoning && (parts.length === 0 || parts[0].search)) {
          out.push({ t: "say", id: nextId(), text: "", reasoning, done: true });
          reasoning = undefined;
        }
        for (const part of parts) {
          if (part.search) {
            out.push({
              t: "tool",
              id: nextId(),
              tool: { id: nextId(), name: "web_search", output: part.text, readOnly: true },
              running: false,
              children: [],
            });
            continue;
          }
          if (!part.text && !reasoning) continue;
          out.push({ t: "say", id: nextId(), text: part.text, reasoning, done: true });
          reasoning = undefined;
        }
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
  // A reload has to fold reads the same way the live stream does, or the same
  // conversation reads differently before and after a reopen.
  let merged: Item[] = [];
  for (const it of out) merged = mergeReads([...merged, it]);
  return { items: merged, plan };
}
