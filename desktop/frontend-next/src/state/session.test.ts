import { describe, expect, it } from "vitest";
import { fromHistory, initialState, reduce, type Item, type SessionEvent, type SessionState } from "./session";
import type { HistoryMessage } from "../port/port";

// vitest runs these in Node, where there is no localStorage. The preference
// reader tolerates its absence by staying off, so a test that wants a receipt
// has to supply one.
const store = new Map<string, string>();
(globalThis as { localStorage?: unknown }).localStorage = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => void store.set(k, v),
  removeItem: (k: string) => void store.delete(k),
};
const showingReceipts = <T,>(body: () => T): T => {
  store.set("rx-turn-receipt", "on");
  try {
    return body();
  } finally {
    store.delete("rx-turn-receipt");
  }
};

const run = (evs: SessionEvent[]): SessionState => evs.reduce(reduce, initialState);
const tools = (s: SessionState) => s.items.filter((i): i is Extract<Item, { t: "tool" }> => i.t === "tool");
const notices = (s: SessionState) =>
  s.items.filter((i) => i.t === "notice").map((i) => (i as Extract<Item, { t: "notice" }>).text);

// The kernel shows a card the moment a call begins, before its arguments have
// finished streaming. Only the full dispatch that follows carries args.
const partial = (id: string): SessionEvent =>
  ({ kind: "tool_dispatch", tool: { id, name: "write_file", partial: true, readOnly: false } }) as SessionEvent;

const full = (id: string): SessionEvent =>
  ({
    kind: "tool_dispatch",
    tool: { id, name: "write_file", args: '{"path":"a.js"}', readOnly: false, added: 3, removed: 0 },
  }) as SessionEvent;

const result = (id: string): SessionEvent =>
  ({
    kind: "tool_result",
    tool: { id, name: "write_file", args: '{"path":"a.js"}', output: "wrote a.js", readOnly: false },
  }) as SessionEvent;

const done = (err?: string): SessionEvent => ({ kind: "turn_done", err }) as SessionEvent;

describe("sealing a turn", () => {
  it("leaves a call that ran and reported alone", () => {
    const s = run([partial("c1"), full("c1"), result("c1"), done()]);
    expect(tools(s)).toHaveLength(1);
    expect(tools(s)[0].tool.err).toBeFalsy();
    expect(notices(s)).toHaveLength(0);
  });

  // The turn was interrupted while the model was still writing the arguments:
  // nothing was dispatched, nothing ran, and no file moved. Saying each of them
  // "reported no result" claims the opposite, and one line per abandoned call
  // buries the reason the turn ended.
  it("folds calls that never started into one line", () => {
    const s = run([partial("c1"), partial("c2"), partial("c3"), done("context canceled")]);
    expect(tools(s)).toHaveLength(0);
    expect(notices(s)).toHaveLength(2);
    expect(notices(s)[0]).toContain("3 个调用");
    expect(notices(s)[1]).toBe("context canceled"); // the kernel's own words, untouched
  });

  it("keeps the old wording for a call that was dispatched and died", () => {
    const s = run([partial("c1"), full("c1"), done("context canceled")]);
    expect(tools(s)).toHaveLength(1);
    expect(tools(s)[0].tool.err).toContain("没有回报结果");
    expect(notices(s)).toHaveLength(1); // only the kernel's error
  });

  it("tells the two apart in the same batch", () => {
    const s = run([partial("c1"), full("c1"), partial("c2"), partial("c3"), done("context canceled")]);
    expect(tools(s)).toHaveLength(1);
    expect(notices(s)[0]).toContain("2 个调用");
  });

  it("reads as one call, not as a count of one", () => {
    const s = run([partial("c1"), done("context canceled")]);
    expect(notices(s)[0]).toBe("还有 1 个调用没来得及开始，它没有改动任何文件");
  });

  // The wire omits `partial: false`, so a fold that merely spread the full
  // dispatch would leave the placeholder's flag standing and every executed
  // call would read as one that never started.
  it("clears the streaming flag once real arguments arrive", () => {
    const progress = { kind: "tool_progress", tool: { id: "c1", name: "bash", output: "…" } } as SessionEvent;
    const s = run([partial("c1"), full("c1"), progress, done("context canceled")]);
    expect(tools(s)).toHaveLength(1);
  });
});

describe("rebuilding a reopened transcript", () => {
  const users = (msgs: HistoryMessage[]) =>
    fromHistory(msgs).items.filter((i): i is Extract<Item, { t: "user" }> => i.t === "user");

  // The control blocks come off before the transcript sees a turn. A turn sent
  // as a dropped file and nothing else still has the "@path" it was referenced
  // by, and keeping that row is how a message that was on screen while you sent
  // it is still there the next time the window opens.
  it("keeps a turn that was an attachment reference and no words", () => {
    const got = users([
      { role: "user", content: "@.reasonix/attachments/clipboard-1.png" },
      { role: "assistant", content: "看到了" },
    ]);
    expect(got).toHaveLength(1);
    expect(got[0].text).toBe("@.reasonix/attachments/clipboard-1.png");
  });

  it("still drops a row that is neither text nor attachment", () => {
    expect(users([{ role: "user", content: "<reasoning-language>zh</reasoning-language>" }])).toHaveLength(0);
  });

  it("leaves an ordinary turn as its text", () => {
    const got = users([{ role: "user", content: "第一句话" }]);
    expect(got).toHaveLength(1);
    expect(got[0].text).toBe("第一句话");
  });
});

describe("the turn's completion receipt", () => {
  const receipts = (s: SessionState) => s.items.filter((i) => i.t === "receipt");
  const finish = (r: unknown): SessionEvent => ({ kind: "turn_done", receipt: r }) as SessionEvent;

  it("keeps the quality summary out of the transcript", () => {
    const s = run([{ kind: "completion_summary", completion: { verdict: "partial", mutations: 2 } } as SessionEvent]);
    expect(s.items).toHaveLength(0);
  });

  it("sources a clean verdict instead of asserting it", () => {
    const s = showingReceipts(() => run([finish({
      verdict: "done", saysSomething: true,
      changes: [{ path: "parser.go", reviewed: false }],
      verifications: [{ command: "go test ./...", passed: true }],
    })]));
    expect(receipts(s)).toHaveLength(1);
  });

  it("shows the turn what it could not verify", () => {
    const s = showingReceipts(() => run([finish({
      verdict: "partial", saysSomething: true,
      gaps: [{ kind: "unreviewed_change", detail: "lexer.go" }],
    })]));
    expect(receipts(s)).toHaveLength(1);
  });

  // The kernel decides; the window does not second-guess it. A receipt with no
  // gaps and no clean verdict says only that the host could not judge, which a
  // transcript that already showed every step does not need repeated.
  it("follows the kernel on what is worth a line", () => {
    expect(receipts(run([finish({ verdict: "incomplete", saysSomething: false })]))).toHaveLength(0);
    expect(receipts(run([done()]))).toHaveLength(0);
  });
});

describe("a notice about the runtime rather than the conversation", () => {
  const notice = (audience: string | undefined, level: string, text: string): SessionEvent =>
    ({ kind: "notice", audience, level, text }) as SessionEvent;

  // The transcript is the record of a conversation. "guardian enabled ·
  // model=deepseek-v4-pro" was the first row of a session nobody had spoken in
  // yet — a fact about the assembly, rendered with the weight of a turn.
  it("keeps an assembly fact out of the transcript", () => {
    const s = run([notice("operator", "info", "guardian enabled · model=x")]);
    expect(notices(s)).toEqual([]);
    expect(s.runtime).toEqual([]);
  });

  // Dropping it entirely would trade one bug for another. A warning is the
  // runtime saying something is wrong with itself, and that has to be seen —
  // just not as something someone said.
  it("gives a runtime warning its own place, not a transcript card", () => {
    const s = run([notice("operator", "warn", "An MCP server failed to start.")]);
    expect(notices(s)).toEqual([]);
    expect(s.runtime.map((n) => n.text)).toEqual(["An MCP server failed to start."]);
  });

  it("lets a runtime warning be dismissed", () => {
    const s = run([notice("operator", "warn", "An MCP server failed to start.")]);
    const gone = reduce(s, { kind: "__runtime_seen", id: s.runtime[0].id } as SessionEvent);
    expect(gone.runtime).toEqual([]);
  });

  // Everything else is still about the conversation and still belongs in it:
  // a permission that was saved, a rewind that was undone, a plan awaiting
  // approval all answer something the user just did.
  it("leaves a conversation notice where it was", () => {
    const s = run([notice(undefined, "info", "undid last rewind")]);
    expect(notices(s)).toEqual(["undid last rewind"]);
    expect(s.runtime).toEqual([]);
  });
});

describe("a line that is still queued", () => {
  const typed = (id: string, text: string): SessionEvent =>
    ({ kind: "__user", text, pending: true, id }) as SessionEvent;
  const rows = (st: SessionState) => st.items.filter((i): i is Extract<Item, { t: "user" }> => i.t === "user");
  const queued = (id: string, itemId: string, kind: "steer" | "followup"): SessionEvent =>
    ({ kind: "__queued", id, itemId, queued: kind }) as SessionEvent;

  // The row is on screen before the kernel has answered. The receipt is what
  // gives it a name to be taken back by, and without it the card has a button
  // it cannot press.
  it("remembers the queue id the kernel answered with", () => {
    const st = run([typed("row-1", "密码我告诉你"), queued("row-1", "inbox-9", "steer")]);
    expect(rows(st)[0].itemId).toBe("inbox-9");
    expect(rows(st)[0].pending).toBe(true);
  });

  // A whole turn refused because one is already running is queued too, and the
  // row has to say which wait it is in: the two land at different moments.
  it("marks a follow-up queued even though it was sent as a turn", () => {
    const sent = ({ kind: "__user", text: "x", pending: false, id: "row-2" }) as SessionEvent;
    const st = run([sent, queued("row-2", "inbox-10", "followup")]);
    expect(rows(st)[0].pending).toBe(true);
    expect(rows(st)[0].queued).toBe("followup");
  });

  it("takes the row away once the kernel drops it", () => {
    const waiting = run([typed("row-1", "x"), queued("row-1", "inbox-9", "steer")]);
    expect(rows(reduce(waiting, { kind: "__unsent", id: "row-1" } as SessionEvent))).toEqual([]);
  });

  // Too late is not an error state on the row: the steer event that made it too
  // late is the same one that stops it calling itself queued.
  it("stops calling itself queued when the turn reads it", () => {
    const waiting = run([typed("row-1", "x"), queued("row-1", "inbox-9", "steer")]);
    const read = reduce(waiting, { kind: "steer", text: "x" } as SessionEvent);
    expect(rows(read)[0].pending).toBe(false);
  });
});

describe("waiting for another session to finish writing", () => {
  const lease = (code: string, level: string, text: string): SessionEvent =>
    ({ kind: "notice", code, level, text }) as SessionEvent;
  const cards = (st: SessionState) => st.items.filter((i): i is Extract<Item, { t: "notice" }> => i.t === "notice");

  // The open used to be the whole surface: one card saying the session would
  // continue "when it is safe", still saying it long after it had.
  it("rewrites the waiting card in place when the wait ends", () => {
    const st = run([
      lease("workspace_lease", "warn", "Another session is writing to this workspace."),
      lease("workspace_lease_resumed", "info", "The workspace is free again; this session has continued."),
    ]);
    expect(cards(st)).toHaveLength(1);
    expect(cards(st)[0].code).toBe("workspace_lease_resumed");
    expect(cards(st)[0].level).toBe("info");
  });

  // A wait is a state, and the strip is where this interface says what a turn
  // is stopped on — the same place 等你批准 goes.
  it("says what the turn is waiting on, and stops saying it when it is not", () => {
    const waiting = run([lease("workspace_lease", "warn", "Another session is writing to this workspace.")]);
    expect(waiting.doing).toBe("等待工作区");
    const gaveUp = reduce(waiting, lease("workspace_lease_abandoned", "info", "The wait ended."));
    expect(gaveUp.doing).toBe("运行中");
    expect(cards(gaveUp)).toHaveLength(1);
  });
});

describe("the turn's verification receipt", () => {
  const done = (receipt: unknown): SessionEvent => ({ kind: "turn_done", receipt }) as SessionEvent;
  const card = { saysSomething: true, verdict: "unproven", gaps: [{ kind: "unverified_change" }] };
  const receipts = (s: SessionState) => s.items.filter((i) => i.t === "receipt");

  // Off unless this machine asked. The card reports what went unverified —
  // worth reading, and easy to tire of after every turn.
  it("stays out of the transcript by default", () => {
    expect(receipts(run([done(card)]))).toEqual([]);
  });

  it("appears when this machine asked for it", () => {
    expect(receipts(showingReceipts(() => run([done(card)])))).toHaveLength(1);
  });

  // The kernel still decides whether a receipt has content at all; the
  // preference only decides whether a reader sees one that does.
  it("still respects the kernel's own answer", () => {
    expect(receipts(showingReceipts(() => run([done({ ...card, saysSomething: false })])))).toEqual([]);
  });
});

// Nothing on the wire echoes what the user typed, so the row is the client's
// to add — and its to take back when the line never left. Reporting the
// refusal alone would leave a turn on screen that never happened.
describe("a line the kernel refused", () => {
  const typed = (id: string, text: string): SessionEvent =>
    ({ kind: "__user", text, pending: false, id }) as SessionEvent;
  const users = (s: SessionState) =>
    s.items.filter((i) => i.t === "user").map((i) => (i as Extract<Item, { t: "user" }>).text);

  it("leaves the transcript", () => {
    const s = run([typed("u1", "first"), typed("u2", "second"), { kind: "__unsent", id: "u2" } as SessionEvent]);
    expect(users(s)).toEqual(["first"]);
  });

  it("takes back only its own row", () => {
    const s = run([typed("u1", "kept"), { kind: "__unsent", id: "nothing-by-that-name" } as SessionEvent]);
    expect(users(s)).toEqual(["kept"]);
  });
});

// The kernel emits Retrying before each backoff sleep and leaves it to the
// window to take the line down again: whatever comes back next is the proof the
// connection recovered, and it is far more often a tool call than a sentence.
describe("the retry line", () => {
  const started = (): SessionEvent => ({ kind: "turn_started" }) as SessionEvent;
  const retrying = (attempt: number, scope?: string): SessionEvent =>
    ({ kind: "retrying", retryAttempt: attempt, retryMax: 5, retryScope: scope }) as SessionEvent;

  it("comes down on the first packet of any kind, not only on text", () => {
    const s = run([started(), retrying(1, "stream"), partial("c1")]);
    expect(s.waiting).toEqual({});
  });

  it("stays up while nothing has come back", () => {
    const s = run([started(), retrying(1, "stream"), { kind: "notice", text: "x" } as SessionEvent]);
    expect(s.waiting.retry).toMatchObject({ attempt: 1, max: 5, scope: "stream" });
  });

  // Only the kernel knows whether it never got a header or lost a body it was
  // already writing out, and the two say different things to the reader.
  it("carries the kernel's own answer for which half broke", () => {
    const s = run([started(), retrying(2, "headers")]);
    expect(s.waiting.retry?.scope).toBe("headers");
  });

  it("shows a stall that starts between two steps", () => {
    const s = run([started(), { kind: "text", text: "hi" } as SessionEvent, retrying(1, "headers")]);
    expect(s.waiting.ttftSince).toBeTruthy();
    expect(s.waiting.retry?.attempt).toBe(1);
  });

  it("times the stall from its first attempt, not its latest", () => {
    const first = run([started(), retrying(1, "stream")]);
    const second = reduce(first, retrying(2, "stream"));
    expect(second.waiting.retry?.since).toBe(first.waiting.retry?.since);
  });

  it("comes down when the turn ends", () => {
    const s = run([started(), retrying(1, "headers"), done()]);
    expect(s.waiting).toEqual({});
  });
});
