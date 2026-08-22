import { describe, expect, it } from "vitest";
import { EN } from "./en";

// The catalogue degrades instead of breaking: a key it does not carry renders as
// Chinese. That is the right failure at runtime and the wrong one to ship, so
// this is what stops an English window from quietly filling with Chinese.
//
// Vite reads the sources, not the filesystem — the frontend carries no Node
// types, and a gate that needed them would only typecheck by accident.
const SOURCES = import.meta.glob(["../**/*.{ts,tsx}", "!../i18n/**", "!../**/*.test.*"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// The first argument of t() or tx(), when it is a plain literal. A key built from a
// conditional is looked up the same way at runtime but cannot be read here.
const CALL = /\b(?:t|tx)\(\s*(["'])((?:\\.|(?!\1)[^\\])*)\1/g;
const HAN = /[一-鿿]/;

describe("English catalogue", () => {
  it("carries every Chinese key the interface asks for", () => {
    const missing: string[] = [];
    for (const [file, body] of Object.entries(SOURCES)) {
      for (const m of body.matchAll(CALL)) {
        const key = m[2];
        if (HAN.test(key) && !(key in EN)) missing.push(`${JSON.stringify(key)} — ${file}`);
      }
    }
    expect([...new Set(missing)], "untranslated keys").toEqual([]);
  });

  // The catalogue can only carry a string the interface asks it for. Chinese
  // written straight into JSX renders as Chinese in an English window and no
  // check sees it, which is how a tab read 新会话 beside a row reading "New
  // session". Text between tags and text in a rendered attribute are the two
  // places that always reach the screen.
  it("routes rendered Chinese through the catalogue", () => {
    const JSX_TEXT = />([^<>{}]*[一-鿿][^<>{}]*)</g;
    const ATTR = /\b(title|placeholder|aria-label|label|alt)="([^"]*[一-鿿][^"]*)"/g;
    const raw: string[] = [];
    for (const [file, body] of Object.entries(SOURCES)) {
      // JSX only exists in .tsx; a ">" inside a plain string is not markup.
      if (!file.endsWith(".tsx")) continue;
      const code = body.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");
      for (const m of code.matchAll(JSX_TEXT)) {
        if (m[1].trim()) raw.push(`${JSON.stringify(m[1].trim())} — ${file}`);
      }
      for (const m of code.matchAll(ATTR)) raw.push(`${m[1]}=${JSON.stringify(m[2])} — ${file}`);
    }
    expect([...new Set(raw)], "rendered Chinese not passed to t()").toEqual([]);
  });

  it("reads the sources it is meant to be guarding", () => {
    expect(Object.keys(SOURCES).length).toBeGreaterThan(30);
  });
});
