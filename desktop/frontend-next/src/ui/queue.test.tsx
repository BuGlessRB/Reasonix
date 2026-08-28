import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Queue } from "./Queue";
import type { Queue as QueueSnapshot, QueueItem } from "../port/port";

const item = (over: Partial<QueueItem> = {}): QueueItem => ({
  id: "i1",
  intent: "steer",
  state: "steer_accepted",
  preview: "把 gate_test.go 里那三个跳过的用例打开",
  createdAt: "2026-08-25T10:00:00Z",
  ...over,
});

const snapshot = (over: Partial<QueueSnapshot> = {}): QueueSnapshot => ({
  revision: 1,
  paused: false,
  items: [item()],
  capacity: { items: 1, maxItems: 64, bytes: 3174, maxBytes: 64 << 20 },
  ...over,
});

const draw = (q: QueueSnapshot) =>
  renderToStaticMarkup(
    <Queue
      queue={q}
      onRead={async () => ""}
      onEdit={() => {}}
      onMove={() => {}}
      onCancel={() => {}}
      onRetry={() => {}}
      onRefresh={() => {}}
      onPause={() => {}}
    />,
  );

// Typing at a working turn does reach the model — the report was that nobody
// could tell. Every assertion here is on what the strip says back, because a
// steer whose only evidence is the model's next sentence reads as ignored.
describe("what the pending strip says back", () => {
  // A background job's follow-on is queued the same way a typed line is, so
  // without saying whose it is the strip reports the runtime as the user.
  it("names a host continuation as the background job it came from", () => {
    const html = draw(snapshot({ items: [item({ origin: "host", state: "queued", intent: "followup" })] }));
    expect(html).toContain("后台任务续接");
    expect(html).not.toContain("插话排队");
  });

  it("names an accepted steer as taken, not as waiting", () => {
    const html = draw(snapshot());
    expect(html).toContain("插话已收");
    expect(html).toContain('data-tone="accent"');
  });

  it("keeps a queued follow-up apart from a steer the kernel took", () => {
    const html = draw(snapshot({ items: [item({ intent: "followup", state: "queued" })] }));
    expect(html).toContain("排队");
    expect(html).not.toContain("插话已收");
  });

  it("offers taking it back while that is still true", () => {
    expect(draw(snapshot())).toContain("取回");
  });

  // Past the tool boundary the text is the model's, and the kernel refuses the
  // withdrawal. Offering it anyway is a button that answers with an error.
  it("drops the take-back once the turn has read the line", () => {
    expect(draw(snapshot({ items: [item({ state: "steer_consumed" })] }))).not.toContain("取回");
  });

  // Both limits refuse on their own, and a strip that showed one of them would
  // be wrong about the other exactly when it bites.
  it("meters count and bytes together", () => {
    const html = draw(snapshot());
    expect(html).toContain("1/64");
    expect(html).toContain("3.1k/64M");
  });

  it("renders nothing when there is nothing waiting and the queue is not held", () => {
    expect(draw(snapshot({ items: [], capacity: { items: 0, maxItems: 64, bytes: 0, maxBytes: 64 << 20 } }))).toBe("");
  });
});
