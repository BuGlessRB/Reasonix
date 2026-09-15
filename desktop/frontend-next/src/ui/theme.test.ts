// @vitest-environment jsdom
import { describe, expect, it, afterEach } from "vitest";
import { apply } from "./theme";
import type { ThemePack } from "../port/port";

const pack: ThemePack = {
  id: "probe",
  name: "Probe",
  tokens: {
    light: { bg: "#FFFFFF", bgSoft: "#F4F4F4", fg: "#1A1A1A", fgDim: "#5A5A5A", fgFaint: "#767676", accent: "#0066CC" },
    dark: { bg: "#101010", bgSoft: "#181818", fg: "#F0F0F0", fgDim: "#B0B0B0", fgFaint: "#8A8A8A", accent: "#66AAFF" },
  },
};

const read = (name: string) => document.documentElement.style.getPropertyValue(name).trim();

afterEach(() => apply(null, "light"));

describe("a pack's inks answer to the reader's contrast", () => {
  // The setting is the reader's, the way reading size is. A pack that pinned
  // --text inline won every step of it, so someone who had asked for stronger
  // text got the author's answer instead for as long as the palette was on.
  it("moves the pack's own colours instead of being overruled by them", () => {
    apply(pack, "light", false, "strong");
    expect(read("--text")).toMatch(/^oklch\(from #1A1A1A calc\(l - 0\.101\) c h\)$/);
    expect(read("--muted")).toContain("#5A5A5A");
    expect(read("--faint")).toContain("#767676");
    expect(read("--ghost")).toBe(read("--faint"));
  });

  it("darkens in light and lightens in dark", () => {
    apply(pack, "light", false, "normal");
    expect(read("--text")).toContain("l - 0.03");
    apply(pack, "dark", false, "normal");
    expect(read("--text")).toContain("l + 0.03");
  });

  // The step a pack states is the one someone who never opened the setting
  // sees, so those two must land on the author's value untouched.
  it("leaves the pack's value alone on the default step", () => {
    apply(pack, "light", false, "");
    expect(read("--text")).toBe("#1A1A1A");
    apply(pack, "light", false, "soft");
    expect(read("--text")).toBe("#1A1A1A");
  });

  it("does not move a surface, only the inks", () => {
    apply(pack, "light", false, "strong");
    expect(read("--page")).toBe("#FFFFFF");
    expect(read("--accent")).toBe("#0066CC");
  });
});
