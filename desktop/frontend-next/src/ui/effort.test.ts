import { describe, expect, it } from "vitest";
import { effortMenu } from "./effort";

describe("a ladder that carries every OpenAI rung", () => {
  const ladder = ["auto", "none", "low", "medium", "high", "xhigh", "max"];
  const rows = effortMenu(ladder, "gpt-5.6-sol", "__declare").filter((row) => ladder.includes(row.value));

  it("names each rung once", () => {
    const labels = rows.map((row) => row.label);
    expect(new Set(labels).size).toBe(labels.length);
  });

  it("shows no reasoning as an empty meter, not as the shallowest depth", () => {
    const none = rows.find((row) => row.value === "none");
    expect(none && "strength" in none ? none.strength : undefined).toBe(0);
  });
});
