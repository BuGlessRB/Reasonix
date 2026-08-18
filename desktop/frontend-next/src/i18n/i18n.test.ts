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

// The first argument of t(), when it is a plain literal. A key built from a
// conditional is looked up the same way at runtime but cannot be read here.
const CALL = /\bt\(\s*(["'])((?:\\.|(?!\1)[^\\])*)\1/g;
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

  it("reads the sources it is meant to be guarding", () => {
    expect(Object.keys(SOURCES).length).toBeGreaterThan(30);
  });
});
