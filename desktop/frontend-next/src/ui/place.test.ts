import { describe, expect, it } from "vitest";
import { placeInViewport } from "./place";

const VIEW = { width: 1000, height: 800 };

// A 232px bubble as it measures at each scale, which is what a rect reports.
const bubble = (scale: number) => ({ width: 232 * scale, height: 100 * scale });

// The invariant the reported bug broke: whatever the scale, the box as rendered
// has to end up inside the viewport. left is written in unscaled pixels, so
// what lands on screen is left * scale.
function rendered(scale: number, wantX: number) {
  const box = bubble(scale);
  const at = placeInViewport({ x: wantX, y: 300 }, box, VIEW, scale);
  return { left: at.left * scale, right: at.left * scale + box.width };
}

describe("placeInViewport", () => {
  it("keeps an edge-anchored bubble inside the viewport at every scale", () => {
    for (const scale of [0.8, 1, 1.15, 1.3, 1.5]) {
      const at = rendered(scale, 900);
      expect(at.right, `scale ${scale} overflows`).toBeLessThanOrEqual(VIEW.width);
      expect(at.left, `scale ${scale} clipped on the left`).toBeGreaterThanOrEqual(0);
    }
  });

  it("changes nothing at the default scale", () => {
    const box = bubble(1);
    expect(placeInViewport({ x: 900, y: 300 }, box, VIEW, 1)).toEqual({
      left: VIEW.width - box.width - 6,
      top: 300,
    });
  });

  it("does not drift with the scale when there is room", () => {
    // Same on-screen anchor, three scales: the bubble lands in the same place
    // on screen rather than sliding right as the interface grows.
    for (const scale of [0.8, 1, 1.5]) {
      expect(rendered(scale, 300).left).toBeCloseTo(300);
    }
  });

  it("clamps the top edge too", () => {
    const at = placeInViewport({ x: 100, y: -40 }, bubble(1.5), VIEW, 1.5);
    expect(at.top * 1.5).toBe(6);
  });
});
