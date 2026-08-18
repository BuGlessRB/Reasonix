import { afterEach, describe, expect, it, vi } from "vitest";
import { SsePort } from "./sse";
import type { WireEvent } from "./wire";

// Driven through the shell's bus, the transport with no reconnect to hide
// behind: every frame is handed over by hand, so a gap is only ever closed by
// the logic under test.
type Feed = (raw: string) => void;
type ReplyBody = { frames?: unknown[]; complete?: boolean } | null;

function attach(reply: (after: number) => ReplyBody = () => ({ frames: [], complete: true })) {
  let feed: Feed = () => {};
  const seen: WireEvent[] = [];
  const asked: string[] = [];
  let gaps = 0;

  vi.stubGlobal("window", {
    runtime: {
      EventsOn: (_name: string, cb: Feed) => {
        feed = cb;
        return () => {};
      },
    },
  });
  vi.stubGlobal("fetch", async (url: string) => {
    asked.push(url);
    if (url.includes("/rx-replay")) return { ok: true, json: async () => ({}) };
    const body = reply(Number(new URL(url, "http://x").searchParams.get("lastEventId") ?? 0));
    if (!body) return { ok: false, status: 500, json: async () => ({}) };
    return { ok: true, json: async () => body };
  });

  const stop = new SsePort("", "r1").subscribe(
    (ev) => seen.push(ev),
    () => {
      gaps++;
    },
  );
  return {
    feed: (kind: string, seq?: number) => feed(JSON.stringify({ kind, ...(seq ? { seq } : {}) })),
    seen,
    kinds: () => seen.map((e) => e.kind),
    replays: () => asked.filter((u) => u.includes("/events/replay")),
    gaps: () => gaps,
    stop,
  };
}

const settle = () => new Promise((r) => setTimeout(r, 0));

afterEach(() => vi.unstubAllGlobals());

describe("recovering the event stream", () => {
  it("passes an unbroken stream straight through", async () => {
    const s = attach();
    s.feed("turn_started", 1);
    s.feed("text");
    s.feed("tool_result", 2);
    await settle();
    expect(s.kinds()).toEqual(["turn_started", "text", "tool_result"]);
    expect(s.replays()).toHaveLength(0);
    expect(s.gaps()).toBe(0);
    s.stop();
  });

  it("fetches exactly what a break in the numbers skipped", async () => {
    const s = attach(() => ({
      frames: [
        { kind: "tool_dispatch", seq: 2 },
        { kind: "approval_request", seq: 3 },
      ],
      complete: true,
    }));
    s.feed("turn_started", 1);
    s.feed("turn_done", 4);
    await settle();
    expect(s.kinds()).toEqual(["turn_started", "tool_dispatch", "approval_request", "turn_done"]);
    expect(s.replays()[0]).toContain("lastEventId=1");
    expect(s.gaps()).toBe(0);
    s.stop();
  });

  // Without this a result reaches the reducer ahead of the dispatch it answers.
  it("holds frames that arrive while a replay is in flight", async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const s = attach();
    vi.stubGlobal("fetch", async (url: string) => {
      await gate;
      const after = Number(new URL(url, "http://x").searchParams.get("lastEventId") ?? 0);
      return { ok: true, json: async () => ({ frames: [{ kind: "tool_dispatch", seq: after + 1 }], complete: true }) };
    });
    s.feed("turn_started", 1);
    s.feed("tool_result", 3); // 2 is missing
    s.feed("turn_done", 4); // arrives mid-recovery
    release();
    await settle();
    await settle();
    expect(s.kinds()).toEqual(["turn_started", "tool_dispatch", "tool_result", "turn_done"]);
    s.stop();
  });

  it("reports a gap the log can no longer close", async () => {
    const s = attach(() => ({ frames: [], complete: false }));
    s.feed("turn_started", 1);
    s.feed("turn_done", 9);
    await settle();
    expect(s.gaps()).toBe(1);
    s.stop();
  });

  // Asking again would not bring the frames back; the transcript is the only
  // source left that can answer.
  it("treats a failed replay request as a gap", async () => {
    const s = attach(() => null);
    s.feed("turn_started", 1);
    s.feed("turn_done", 9);
    await settle();
    expect(s.gaps()).toBe(1);
    s.stop();
  });

  it("notices a frame lost at the end of a turn", async () => {
    const s = attach(() => ({ frames: [{ kind: "turn_done", seq: 2 }], complete: true }));
    s.feed("turn_started", 1);
    s.feed("stream_watermark", 2);
    await settle();
    expect(s.kinds()).toContain("turn_done");
    expect(s.kinds()).not.toContain("stream_watermark");
    s.stop();
  });

  it("stays quiet when the watermark matches what it holds", async () => {
    const s = attach();
    s.feed("turn_done", 1);
    s.feed("stream_watermark", 1);
    await settle();
    expect(s.replays()).toHaveLength(0);
    s.stop();
  });

  it("takes the server's own gap signal without rendering it", async () => {
    const s = attach();
    s.feed("stream_gap", 7);
    await settle();
    expect(s.gaps()).toBe(1);
    expect(s.seen).toHaveLength(0);
    s.stop();
  });

  // A restarted server counts from one again. Comparing against the watermark
  // from the process before would silently disable every later gap check.
  it("reads numbering that goes backwards as a new stream", async () => {
    const s = attach(() => ({ frames: [{ kind: "tool_dispatch", seq: 3 }], complete: true }));
    s.feed("turn_started", 40);
    s.feed("turn_started", 1); // server restarted
    s.feed("tool_result", 2);
    s.feed("turn_done", 9); // a real gap, on the new numbering
    await settle();
    expect(s.gaps()).toBeGreaterThanOrEqual(1);
    expect(s.replays().some((u) => u.includes("lastEventId=2"))).toBe(true);
    s.stop();
  });
});
