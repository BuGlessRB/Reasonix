import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { EN } from "./en";

// The catalogue degrades instead of breaking: a key it does not carry renders as
// Chinese. That is the right failure at runtime and the wrong one to ship, so
// this is what stops an English window from quietly filling with Chinese.

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function sources(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) return sources(p);
    return /\.tsx?$/.test(e.name) && !/\.test\./.test(e.name) ? [p] : [];
  });
}

// The first argument of t(), when it is a plain literal. A key built from a
// conditional is looked up the same way at runtime but cannot be read here.
const CALL = /\bt\(\s*(["'])((?:\\.|(?!\1)[^\\])*)\1/g;
const HAN = /[一-鿿]/;

function keysUsed(): Map<string, string[]> {
  const used = new Map<string, string[]>();
  for (const file of sources(SRC)) {
    if (file.startsWith(path.join(SRC, "i18n"))) continue;
    const body = fs.readFileSync(file, "utf8");
    for (const m of body.matchAll(CALL)) {
      const where = path.relative(SRC, file);
      const at = used.get(m[2]);
      if (at) at.push(where);
      else used.set(m[2], [where]);
    }
  }
  return used;
}

describe("English catalogue", () => {
  it("carries every Chinese key the interface asks for", () => {
    const missing = [...keysUsed()]
      .filter(([key]) => HAN.test(key) && !(key in EN))
      .map(([key, files]) => `${JSON.stringify(key)} — ${[...new Set(files)].join(", ")}`);
    expect(missing, `untranslated:\n${missing.join("\n")}`).toEqual([]);
  });
});
