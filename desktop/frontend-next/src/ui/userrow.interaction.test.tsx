// @vitest-environment jsdom
import "./testkit";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Item } from "../state/session";
import type { Checkpoint } from "../port/port";
import { UserCard } from "./cards/UserCard";

afterEach(cleanup);

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

describe("the controls below a message", () => {
  it("copies the user message verbatim and reports success", async () => {
    const user = userEvent.setup();
    const write = vi.spyOn(navigator.clipboard, "writeText");
    const text = "第一行\n  第二行";
    row(text);
    const copy = screen.getByRole("button", { name: "复制" });
    await user.click(copy);
    expect(write).toHaveBeenCalledExactlyOnceWith(text);
    expect(copy.getAttribute("title")).toBe("已复制");
    write.mockRestore();
  });

  it("shows icons with accessible names and hints for a short message", () => {
    row("好");
    const edit = screen.getByRole("button", { name: "改写" });
    const back = screen.getByRole("button", { name: "回到这里" });
    expect(edit.textContent).toBe("");
    expect(back.textContent).toBe("");
    expect(edit.title).toBe("改写这条消息并重新发送");
    expect(back.title).toBe("将工作区与对话回退至该消息之前");
    expect(screen.getByRole("button", { name: "复制" }).querySelector(".sr-only")).not.toBeNull();
  });

  it("keeps the same icon order for a long message", () => {
    row("请把 internal/net 里的重试逻辑改成指数退避，最多重试五次，并补一条单元测试。");
    const edit = screen.getByRole("button", { name: "改写" });
    const back = screen.getByRole("button", { name: "回到这里" });
    expect(edit.textContent).toBe("");
    expect(back.textContent).toBe("");
    expect(screen.getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["复制", "改写", "回到这里"]);
    expect(screen.getByRole("button", { name: "复制" }).querySelector(".sr-only")).not.toBeNull();
  });

  it("stay reachable from the keyboard as icons", async () => {
    row("好");
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "复制" }));
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "改写" }));
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "回到这里" }));
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("menu")).toBeTruthy();
  });
});
