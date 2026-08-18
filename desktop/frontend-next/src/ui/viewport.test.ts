import { describe, expect, it } from "vitest";
import { foldsAt } from "./viewport";

describe("layout folds", () => {
  it("keeps both columns when there is room", () => {
    expect(foldsAt(1920)).toBe("");
    expect(foldsAt(1201)).toBe("");
  });

  it("folds on the boundary, the way the media query it replaced did", () => {
    expect(foldsAt(1200)).toBe("rail");
    expect(foldsAt(840)).toBe("rail side");
    expect(foldsAt(640)).toBe("rail side scene");
  });

  it("accumulates, so a narrower window never un-folds a wider one's column", () => {
    const widths = [1920, 1200, 900, 840, 700, 640, 320];
    const counts = widths.map((w) => (foldsAt(w) === "" ? 0 : foldsAt(w).split(" ").length));
    expect(counts).toEqual([...counts].sort((a, b) => a - b));
  });

  // The zoom setting scales the interface without moving the window, so this is
  // asked about the room the layout has rather than the room the display has.
  it("folds a 1920 window once it is zoomed past its room", () => {
    expect(foldsAt(1920 / 1.3)).toBe("");
    expect(foldsAt(1920 / 1.7)).toBe("rail");
    expect(foldsAt(1920 / 2.4)).toBe("rail side");
  });
});
