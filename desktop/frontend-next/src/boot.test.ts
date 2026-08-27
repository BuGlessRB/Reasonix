import { describe, expect, it } from "vitest";
import HTML from "../index.html?raw";

// The rules the boot screen carries paint before the bundle that would style
// anything exists, so nothing else is watching them: perf.html is a separate
// entry, and every browser guard under perf/ loads that one.
describe("the shipped entry", () => {
  const inline = HTML.slice(HTML.indexOf("<style>"), HTML.lastIndexOf("</style>"));

  it("leaves the root element's background alone", () => {
    // A theme's picture is body::before at z-index -2, visible only because an
    // uncoloured html hands the canvas to body's background. Colouring html
    // takes the canvas over and demotes body's to an ordinary element paint,
    // which lands on top of the picture.
    const offenders: string[] = [];
    for (const rule of inline.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      const selectors: string = rule[1];
      const declarations: string = rule[2];
      const onRoot = selectors.split(",").some((one: string) => /(^|\s)(html|:root)\s*$/.test(one));
      if (onRoot && /(^|;)\s*background(-color)?\s*:/.test(declarations)) offenders.push(selectors.trim());
    }
    expect(offenders, "html/:root must not set a background").toEqual([]);
  });

  it("paints the boot screen itself opaque", () => {
    expect(inline).toMatch(/\.boot\s*\{[^}]*background:\s*var\(--boot-bg\)/);
  });
});
