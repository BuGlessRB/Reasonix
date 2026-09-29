// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import type { Item } from "../../state/session";
import { AskCard } from "./AskCard";

afterEach(cleanup);

it("formats the ask prompt while preserving its line breaks", async () => {
  const item = {
    t: "ask",
    id: "row",
    ask: {
      id: "call",
      questions: [{
        id: "q1",
        header: "选择",
        prompt: "第一行\n第二行 **重点**",
        options: [{ label: "方案 A" }],
      }],
    },
  } as Extract<Item, { t: "ask" }>;

  const { container } = render(<AskCard item={item} onAnswer={vi.fn()} />);
  await waitFor(() => expect(container.querySelector(".ask-q strong")?.textContent).toBe("重点"));
  expect(container.querySelector(".ask-q .md")?.textContent).toContain("第一行\n第二行");
});
