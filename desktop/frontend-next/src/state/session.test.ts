import { describe, expect, it } from "vitest";
import { fromHistory, initialState, reduce, type Item, type SessionEvent, type SessionState } from "./session";
import type { HistoryMessage } from "../port/port";

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
    const s = run([finish({
      verdict: "done", saysSomething: true,
      changes: [{ path: "parser.go", reviewed: false }],
      verifications: [{ command: "go test ./...", passed: true }],
    })]);
    expect(receipts(s)).toHaveLength(1);
  });

  it("shows the turn what it could not verify", () => {
    const s = run([finish({
      verdict: "partial", saysSomething: true,
      gaps: [{ kind: "unreviewed_change", detail: "lexer.go" }],
    })]);
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
