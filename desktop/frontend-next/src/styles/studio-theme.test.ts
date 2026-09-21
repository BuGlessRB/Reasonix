import { describe, expect, it } from "vitest";

const CSS = import.meta.glob("./studio.css", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const MARKER = "Theme token closure";

describe("the Studio shell closes over the active theme", () => {
  const css = Object.values(CSS)[0];
  const at = css.lastIndexOf(MARKER);
  const closure = at >= 0 ? css.slice(at) : "";

  it("keeps the compatibility layer last so prototype literals cannot win later", () => {
    expect(at).toBeGreaterThan(0);
    expect(css.slice(at + MARKER.length)).not.toContain(MARKER);
  });

  it("covers every persistent product surface", () => {
    for (const selector of [
      ".chrome", ".rail", ".compose", ".activity-group", ".studio-browser", ".btn.send",
      ".prefs-sheet", ".prefs-nav", ".prefs-main", ".grp",
    ]) {
      expect(closure, `${selector} is outside the theme closure`).toContain(selector);
    }
  });

  it("uses tokens instead of adding another fixed palette", () => {
    expect(closure).not.toMatch(/#[0-9a-f]{3,8}\b/i);
    for (const token of ["--page", "--surface", "--raised", "--overlay", "--border", "--text", "--muted", "--accent"]) {
      expect(closure, `${token} is not consumed by the closure`).toContain(`var(${token})`);
    }
  });

  it("keeps conversation selection quiet instead of painting an accent rail", () => {
    const selected = closure.match(/\.rail \.sessrow\[aria-selected="true"\]\s*\{([^}]*)\}/s)?.[1] ?? "";
    expect(selected).toContain("var(--overlay)");
    expect(selected).not.toContain("var(--accent-wash)");
    expect(selected).not.toContain("var(--accent)");
    expect(selected).toContain("box-shadow: none");
  });

  it("pins the composer to the Studio type scale even when a theme pack is active", () => {
    expect(closure).toMatch(/\.compose textarea\s*\{[^}]*font:\s*400 13px\/1\.65 var\(--ui\)/s);
    expect(closure).toMatch(/\.compose textarea::placeholder\s*\{[^}]*font:\s*inherit/s);
    expect(closure).toMatch(/\.compose \.queue\s*\{[^}]*font:\s*500 10px\/1\.35 var\(--ui\)/s);
    expect(closure).toMatch(/\.compose \.mode,[\s\S]*?font-size:\s*11px !important/s);
  });

  it("keeps the sidebar brand on the same title-bar row as the chrome", () => {
    expect(closure).toMatch(/\.studio-brand\s*\{[^}]*height:\s*var\(--chrome-h\)/s);
    expect(closure).toMatch(/\.studio-collapse\s*\{[^}]*width:\s*30px[^}]*height:\s*30px/s);
  });

  it("gives light-theme run data and composer metrics their own readable surfaces", () => {
    expect(closure).toMatch(/\.studio-meterrail\s*\{[^}]*background:\s*color-mix\([^}]*var\(--surface\)/s);
    expect(closure).toMatch(/:root\[data-theme="light"\] \.studio-meterrail\s*\{[^}]*var\(--float\)/s);
    expect(closure).toMatch(/\.scroll\[data-pane="analysis"\]\s*\{[^}]*background:\s*transparent/s);
    expect(closure).toMatch(/\.run-budget, \.run-signals, \.run-rounds\s*\{[^}]*background:[^}]*var\(--raised\)/s);
  });

  it("keeps an inspected activity row neutral instead of using an AI-like status glow", () => {
    const selected = closure.match(/\.run-rounds li\[data-on\]\s*\{([^}]*)\}/s)?.[1] ?? "";
    expect(selected).toContain("var(--text)");
    expect(selected).toContain("var(--muted)");
    expect(selected).not.toContain("var(--ra-recovery)");
    expect(selected).not.toContain("var(--accent)");
  });
});
