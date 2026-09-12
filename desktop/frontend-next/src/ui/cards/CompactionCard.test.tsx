import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { CompactionCard } from "./CompactionCard";
import type { Compaction } from "../../port/wire";

const draw = (c: Compaction) => renderToStaticMarkup(<CompactionCard c={c} done />);

// The card said "a threshold was reached" and stopped there. Against a declared
// 1M window that is a fold at 16% with no reason on screen, and the reader
// supplies the only one available: that the host folds whenever it feels like
// it. The host knows which bound fired and how big it is — this is where it says so.
describe("what the card says sent a fold", () => {
  it("names the fixed bound when that is the one that fired", () => {
    const html = draw({ trigger: "auto", boundary: "economic", triggerTokens: 160_000, messages: 40 });
    expect(html).toContain("固定维护线");
    expect(html).toContain("160k");
  });

  it("names the window's own bound when that one fired instead", () => {
    const html = draw({ trigger: "auto", boundary: "capacity", triggerTokens: 108_800, messages: 40 });
    expect(html).toContain("窗口容量线");
    expect(html).toContain("109k");
  });

  // A replayed session recorded before the boundary rode the event has no
  // boundary to name, and inventing one would put a threshold on screen that
  // nothing measured.
  it("falls back to the old wording rather than guessing a bound", () => {
    const html = draw({ trigger: "auto", messages: 40 });
    expect(html).toContain("上下文达到阈值");
    expect(html).not.toContain("维护线");
  });

  // A fold the user asked for needs no threshold at all: they are the reason.
  it("says nothing about thresholds for a fold the user asked for", () => {
    const html = draw({ trigger: "manual", boundary: "economic", triggerTokens: 160_000, messages: 40 });
    expect(html).toContain("手动触发");
    expect(html).not.toContain("160k");
  });
});
