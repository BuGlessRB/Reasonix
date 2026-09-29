// @vitest-environment jsdom
import "./testkit";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Item } from "../state/session";
import type { Checkpoint } from "../port/port";
import { UserCard } from "./cards/UserCard";

afterEach(cleanup);

// jsdom lays nothing out, so this file supplies the one fact the row reads: the
// bubble is as wide as its text and a control as wide as its label. Whether the
// browser agrees is what the screenshot pass checks; this guards what the row
// does with the answer.
const GLYPH = 14;
const widths = {
  offsetWidth(this: HTMLElement) {
    const icon = this.querySelector("svg") ? 12 : 0;
    return (this.textContent ?? "").length * GLYPH + icon + 16;
  },
  clientWidth(this: HTMLElement) {
    if (!this.classList.contains("user-hl")) return 0;
    const bubble = this.closest(".c")?.querySelector(".out .txt");
    return (bubble?.textContent ?? "").length * GLYPH + 30;
  },
};
const saved = Object.keys(widths).map((k) => [k, Object.getOwnPropertyDescriptor(HTMLElement.prototype, k)] as const);
beforeAll(() => {
  for (const [k, get] of Object.entries(widths)) Object.defineProperty(HTMLElement.prototype, k, { configurable: true, get });
});
afterAll(() => {
  for (const [k, d] of saved) if (d) Object.defineProperty(HTMLElement.prototype, k, d);
});

const cp: Checkpoint = { turn: 4, prompt: "", files: 2, msgIndex: 7 };
const row = (text: string) => {
  const item = { t: "user", id: "row", text } as Extract<Item, { t: "user" }>;
  const never = () => new Promise<never>(() => {});
  render(
    <UserCard
      item={item}
      cp={cp}
      onResend={vi.fn()}
      onPrepareRewind={vi.fn(never)}
      onCommitRewind={vi.fn(never)}
      onUndoRewind={vi.fn(never)}
    />,
  );
};

describe("the controls above a message", () => {
  it("fall back to icons over a bubble narrower than their labels, keeping names and hints", () => {
    row("好");
    const edit = screen.getByRole("button", { name: "改写" });
    const back = screen.getByRole("button", { name: "回到这里" });
    expect(edit.textContent).toBe("");
    expect(back.textContent).toBe("");
    expect(edit.title).toBe("改写这条消息并重新发送");
    expect(back.title).toBe("将工作区与对话回退至该消息之前");
  });

  it("keep their labels over a bubble wide enough for them", () => {
    row("请把 internal/net 里的重试逻辑改成指数退避，最多重试五次，并补一条单元测试。");
    const edit = screen.getByRole("button", { name: "改写" });
    const back = screen.getByRole("button", { name: "回到这里" });
    expect(edit.textContent).toBe("改写");
    expect(back.textContent).toBe("回到这里");
    expect(edit.getAttribute("aria-label")).toBeNull();
    expect(back.getAttribute("aria-label")).toBeNull();
  });

  it("stay reachable from the keyboard as icons", async () => {
    row("好");
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "改写" }));
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "回到这里" }));
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("menu")).toBeTruthy();
  });
});
