import { describe, expect, it } from "vitest";

const SHEETS = import.meta.glob(["./app.css", "./studio.css"], { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const SELECTOR = ".compose .row .go";

interface Rule {
  maxWidth: number;
  decls: Map<string, string>;
}

function blockEnd(css: string, open: number): number {
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css[i] === "{") depth++;
    else if (css[i] === "}" && --depth === 0) return i;
  }
  return css.length;
}

function declarations(body: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const part of body.split(";")) {
    const at = part.indexOf(":");
    if (at > 0) out.set(part.slice(0, at).trim(), part.slice(at + 1).trim());
  }
  return out;
}

// The rules for the send cluster in source order, each with the widest
// conversation column it applies to (Infinity outside a container query).
function rules(css: string): Rule[] {
  const out: { at: number; rule: Rule }[] = [];
  const containers: { from: number; to: number; maxWidth: number }[] = [];
  for (const m of css.matchAll(/@container conversation \(max-width:\s*(\d+)px\)\s*\{/g)) {
    const open = m.index + m[0].length - 1;
    containers.push({ from: open, to: blockEnd(css, open), maxWidth: Number(m[1]) });
  }
  const escaped = SELECTOR.replace(/\./g, "\\.");
  for (const m of css.matchAll(new RegExp(`(?:^|[}\\s])${escaped}\\s*\\{([^}]*)\\}`, "g"))) {
    const inside = containers.find((c) => m.index > c.from && m.index < c.to);
    out.push({ at: m.index, rule: { maxWidth: inside?.maxWidth ?? Infinity, decls: declarations(m[1]) } });
  }
  return out.sort((a, b) => a.at - b.at).map((r) => r.rule);
}

describe("the composer's send cluster on a narrow column", () => {
  const ordered = [...rules(SHEETS["./app.css"]), ...rules(SHEETS["./studio.css"])];

  const computed = (width: number) => {
    const out = new Map<string, string>();
    for (const rule of ordered) {
      if (width > rule.maxWidth) continue;
      for (const [k, v] of rule.decls) out.set(k, v);
    }
    return out;
  };

  it("reads the rules it judges", () => {
    expect(ordered.length).toBeGreaterThan(1);
    expect([320, 358, 400, 460].some((w) => computed(w).get("position") === "absolute")).toBe(true);
  });

  // Out of flow and a full row wide, the cluster sits on top of the model and
  // effort pickers that share its line, and every tap lands on it instead.
  it("never lies across the whole row once it is taken out of flow", () => {
    for (const width of [300, 320, 358, 390, 420, 460, 470]) {
      const style = computed(width);
      if (style.get("position") !== "absolute") continue;
      expect(style.get("width") ?? "auto", `at a ${width}px column`).not.toBe("100%");
    }
  });
});
