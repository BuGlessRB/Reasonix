import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Plan } from "./Plan";
import type { PlanStep } from "../state/session";

const steps = (...done: boolean[]): PlanStep[] => done.map((d, i) => ({ text: `第 ${i + 1} 步`, done: d }));

describe("the plan block", () => {
  // No plan and an empty plan are the same absence. A block reading "0 / 0 ·
  // 尚未制定" is a wall to skip on every glance at a rail that already has nine.
  it("draws nothing when there is no plan", () => {
    expect(renderToStaticMarkup(<Plan steps={[]} />)).toBe("");
  });

  it("counts what is done against what there is", () => {
    const html = renderToStaticMarkup(<Plan steps={steps(true, true, false, false)} />);
    expect(html).toContain('data-b="plan"');
    expect(html).toContain("2 / 4");
    expect(html).toContain("width:50%");
  });

  // The first unfinished step is where the run is, and it is the only one the
  // cursor and the weight point at.
  it("marks exactly one step as the one happening now", () => {
    const html = renderToStaticMarkup(<Plan steps={steps(true, false, false)} />);
    expect(html.match(/data-now=""/g)).toHaveLength(1);
    expect(html.match(/data-done=""/g)).toHaveLength(1);
  });

  // Every step done means nothing is happening now — not that the last one is.
  it("points at no step once they are all done", () => {
    expect(renderToStaticMarkup(<Plan steps={steps(true, true)} />)).not.toContain('data-now');
  });

  // todo_write rewrites the whole list every call, so two steps with the same
  // words must still be two rows rather than one React key collision.
  it("keeps duplicate steps apart", () => {
    const same: PlanStep[] = [{ text: "跑测试", done: true }, { text: "跑测试", done: false }];
    expect(renderToStaticMarkup(<Plan steps={same} />).match(/class="s"/g)).toHaveLength(2);
  });
});
