import { describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({ lang: "zh" as "zh" | "en" }));
vi.mock("./index", () => ({ current: () => state.lang }));

import { bytes, category, count, decimals, money, pct, seconds, tokens } from "./format";

function inEnglish<T>(fn: () => T): T {
  state.lang = "en";
  try {
    return fn();
  } finally {
    state.lang = "zh";
  }
}

describe("count", () => {
  // The English window had no separator anywhere before this: every large
  // number was printed by interpolating the raw value.
  it("groups thousands", () => {
    expect(count(1234567)).toBe("1,234,567");
    expect(inEnglish(() => count(1234567))).toBe("1,234,567");
  });

  it("leaves small numbers alone", () => {
    expect(count(0)).toBe("0");
    expect(count(999)).toBe("999");
  });
});

describe("tokens", () => {
  // The width has to stay put as a turn grows, which is why the decimal is
  // dropped once the number reaches five figures.
  it("keeps one decimal under ten thousand and none above", () => {
    expect(tokens(999)).toBe("999");
    expect(tokens(1200)).toBe("1.2k");
    expect(tokens(9999)).toBe("10k");
    expect(tokens(12_345)).toBe("12k");
    expect(tokens(123_456)).toBe("123k");
  });

  it("switches to millions", () => {
    expect(tokens(1_200_000)).toBe("1.2M");
    expect(tokens(12_000_000)).toBe("12.0M");
  });

  // The scale a token count is read on, not a quantity feeling: Intl's compact
  // notation would say 1.2万 here, which is not what the field measures.
  it("uses the same suffix in both languages", () => {
    expect(inEnglish(() => tokens(1200))).toBe("1.2k");
  });
});

describe("seconds", () => {
  it("spells one place by default", () => {
    expect(seconds(1500)).toBe("1.5s");
    expect(seconds(500)).toBe("0.5s");
  });

  it("takes the precision its caller needs", () => {
    expect(seconds(1500, 0)).toBe("2s");
    expect(seconds(1234, 2)).toBe("1.23s");
  });
});

describe("bytes", () => {
  it("steps by binary units", () => {
    expect(bytes(512)).toBe("512 B");
    expect(bytes(2048)).toBe("2 KB");
    expect(bytes(300_000)).toBe("293 KB");
    expect(bytes(5 << 20)).toBe("5.0 MB");
  });
});

describe("pct", () => {
  it("takes a ratio, not a percentage", () => {
    expect(pct(0.5)).toBe("50%");
    expect(pct(0.9982, 2)).toBe("99.82%");
  });
});

describe("money", () => {
  // The whole reason to hand this to Intl: in an English window ¥ alone does
  // not say which yuan or yen it is, and a symbol glued to the front cannot
  // make that distinction.
  it("distinguishes the yuan from the yen when the reader needs it", () => {
    expect(money(12.34, "CNY")).toBe("¥12.34");
    expect(inEnglish(() => money(12.34, "CNY"))).toBe("CN¥12.34");
  });

  it("places the symbol where the language puts it", () => {
    expect(inEnglish(() => money(1234.5, "USD"))).toBe("$1,234.50");
  });

  // An ISO-shaped code Intl does not know is still formatted by it, with the
  // code standing in for the symbol. Only the spacing is ICU's business, so the
  // assertion does not pin it.
  it("keeps an unknown code rather than dropping it", () => {
    expect(money(5, "XYZ")).toMatch(/^XYZ\s?5\.00$/);
  });

  // A bare symbol is not a code, so it takes the fallback rather than being
  // handed to Intl as one.
  it("still renders a legacy symbol", () => {
    expect(money(5, "¥")).toBe("¥5.00");
  });
});

describe("category", () => {
  // English needs two forms and Chinese one, but asking the language means a
  // catalogue that later grows Russian's four does not need this rewritten.
  it("reports the language's own plural categories", () => {
    expect(category(1)).toBe("other");
    expect(inEnglish(() => category(1))).toBe("one");
    expect(inEnglish(() => category(2))).toBe("other");
    expect(inEnglish(() => category(0))).toBe("other");
  });
});

describe("decimals", () => {
  it("keeps exactly the places asked for", () => {
    expect(decimals(1, 2)).toBe("1.00");
    expect(decimals(1.005, 2)).toBe("1.01");
  });
});
