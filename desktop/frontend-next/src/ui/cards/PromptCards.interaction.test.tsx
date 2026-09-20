// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Item } from "../../state/session";
import { ApprovalCard } from "./ApprovalCard";
import { AskCard } from "./AskCard";

afterEach(cleanup);

const pending = () => new Promise<void>((resolve) => setTimeout(resolve, 20));

describe("decision cards", () => {
  it("keeps every approval action locked while the decision is in flight", async () => {
    const item = {
      t: "approval", id: "row", a: { id: "gate", tool: "bash", subject: "run checks" },
    } as Extract<Item, { t: "approval" }>;
    let release = () => {};
    const approve = vi.fn(() => new Promise<void>((resolve) => { release = resolve; }));
    render(<ApprovalCard item={item} onApprove={approve} onPlan={vi.fn(pending)} />);

    await userEvent.click(screen.getByRole("button", { name: "允许这一次" }));
    expect(screen.getByText("正在提交…")).toBeTruthy();
    for (const button of screen.getAllByRole("button")) expect((button as HTMLButtonElement).disabled).toBe(true);
    release();
    await waitFor(() => expect((screen.getByRole("button", { name: "允许这一次" }) as HTMLButtonElement).disabled).toBe(false));
    expect(approve).toHaveBeenCalledTimes(1);
  });

  // An answer the host drops must not be offered: the card used to promise "do
  // not ask again" and send a grant that died with the session, and the grant
  // that actually writes a rule was not reachable from this window at all.
  it("offers the answers this call's host says it will honour, and no others", async () => {
    const card = (allows: { allowsSession?: boolean; allowsPersist?: boolean }) => {
      const approve = vi.fn(pending);
      const item = {
        t: "approval", id: "row", a: { id: "gate", tool: "computer_act", subject: "com.apple.Notes", ...allows },
      } as Extract<Item, { t: "approval" }>;
      const r = render(<ApprovalCard item={item} onApprove={approve} onPlan={vi.fn(pending)} />);
      return { approve, ...r };
    };
    const names = () => screen.getAllByRole("button").map((b) => b.textContent);

    const fresh = card({});
    expect(names()).toEqual(["允许这一次", "拒绝"]);
    cleanup();

    const scoped = card({ allowsSession: true });
    expect(names()).toEqual(["允许这一次", "本会话都允许", "拒绝"]);
    await userEvent.click(screen.getByRole("button", { name: "本会话都允许" }));
    expect(scoped.approve).toHaveBeenCalledWith("row", "gate", "session");
    cleanup();

    const full = card({ allowsSession: true, allowsPersist: true });
    expect(names()).toEqual(["允许这一次", "本会话都允许", "此类操作不再询问", "拒绝"]);
    await userEvent.click(screen.getByRole("button", { name: "此类操作不再询问" }));
    expect(full.approve).toHaveBeenCalledWith("row", "gate", "always");
    void fresh;
  });

  it("does not invent a recommendation and restores answered tab state", () => {
    const item = {
      t: "ask", id: "row", answered: [["B"]], ask: { id: "ask", questions: [
        { id: "q", header: "方向", prompt: "选哪个？", multi: false, options: [{ label: "A" }, { label: "B" }] },
      ] },
    } as Extract<Item, { t: "ask" }>;
    const { container } = render(<AskCard item={item} onAnswer={vi.fn(pending)} />);
    expect(screen.queryByText("推荐")).toBeNull();
    expect(screen.getByRole("button", { name: "B" }).getAttribute("aria-pressed")).toBe("true");
    expect(within(container).getByText(/方向：/).parentElement?.textContent).toContain("B");
  });
});
