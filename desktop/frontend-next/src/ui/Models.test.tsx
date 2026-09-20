import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ModelEntry } from "../port/port";
import { Models } from "./Models";

const MODELS: ModelEntry[] = [
  {
    ref: "deepseek/deepseek-flash",
    provider: "deepseek",
    vendor: "api.deepseek.com",
    model: "deepseek-flash",
    kind: "anthropic",
    vision: true,
    efforts: ["auto", "high"],
    contextWindow: 1_000_000,
    price: { input: 0.15, output: 0.6, currency: "$" },
  },
];

describe("model settings list", () => {
  it("keeps the provider visible and explains the selected model's metadata", () => {
    const html = renderToStaticMarkup(
      <Models models={MODELS} current={MODELS[0].ref} busy="" protocol={{}} onPick={() => undefined} />,
    );

    expect(html).toContain("api.deepseek.com");
    expect(html).toContain("Anthropic 兼容");
    expect(html).toContain("当前使用");
    expect(html).toContain("上下文 1M");
    expect(html).toContain("输入 $0.15 · 输出 $0.60 / 1M tokens");
  });
});
