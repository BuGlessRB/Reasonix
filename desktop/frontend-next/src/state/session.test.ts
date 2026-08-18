import { describe, expect, it } from "vitest";
import { initialState, reduce, type Item, type SessionEvent, type SessionState } from "./session";

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
