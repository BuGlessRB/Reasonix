import { describe, expect, it } from "vitest";
import { ISOLATED } from "./isolated";

const SOURCES = import.meta.glob(["../**/*.test.ts", "../**/*.test.tsx"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// The list the config reads is one place; which files actually mock a module is
// another. A file that mocks and is not listed runs in the shared worker, where
// its mock does nothing and it passes alone but fails in the suite — the exact
// failure this pairing exists to keep from being discovered by accident.
describe("the isolated test project", () => {
  const mockers = Object.entries(SOURCES)
    .filter(([, src]) => src.includes("vi.mock("))
    .map(([path]) => path.replace(/^\.\.\//, "src/"))
    .sort();

  it("covers every file that mocks a module, and nothing else", () => {
    expect(mockers, "vi.mock without a line in src/testing/isolated.ts, or a line with no vi.mock behind it").toEqual([...ISOLATED].sort());
  });
});
