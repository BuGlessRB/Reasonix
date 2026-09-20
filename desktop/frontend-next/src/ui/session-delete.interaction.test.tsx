// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Workspaces } from "./Workspaces";
import { MockHub } from "../port/mock_hub";
import type { HubPort, RuntimeView, TreeWorkspace } from "../port/hub";

afterEach(cleanup);

const SESSION = "/w/.reasonix/sessions/20260901-120000.jsonl";

function tree(over: Partial<TreeWorkspace> = {}): TreeWorkspace[] {
  return [
    {
      root: "/w",
      name: "w",
      sessions: [{ path: SESSION, name: "20260901-120000", title: "the one to delete" }],
      ...over,
    } as TreeWorkspace,
  ];
}

function draw(over: { runtimes?: RuntimeView[]; workspaces?: TreeWorkspace[] } = {}) {
  const hub = new MockHub() as unknown as HubPort;
  const removeSession = vi.spyOn(hub, "removeSession").mockResolvedValue(undefined);
  const onClose = vi.fn().mockResolvedValue(undefined);
  const reload = vi.fn().mockResolvedValue(undefined);
  const onError = vi.fn();
  render(
    <Workspaces
      hub={hub}
      tree={over.workspaces ?? tree()}
      runtimes={over.runtimes ?? []}
      active=""
      folded={new Set()}
      onFold={() => {}}
      reload={reload}
      onOpen={async () => {}}
      onFocus={() => {}}
      onClose={onClose}
      liveIds={() => []}
      onRename={() => {}}
      onError={onError}
      adder={{ add: () => {}, close: () => {}, at: null } as never}
    />,
  );
  return { removeSession, onClose, reload, onError };
}

const trash = async () => {
  await userEvent.click(screen.getByRole("button", { name: /会话操作：/ }));
  return screen.getByRole("menuitem", { name: "删除会话" });
};
const confirmGo = () => within(screen.getByRole("alertdialog")).getByRole("button", { name: "删除" });

describe("deleting a conversation from the rail", () => {
  // The whole gesture end to end: the row, the trash, and the confirmation that
  // replaces the row.
  it("asks, then deletes what it asked about", async () => {
    const { removeSession, reload } = draw();
    await userEvent.click(await trash());
    expect(screen.getByRole("alertdialog")).toBeTruthy();

    await userEvent.click(confirmGo());
    expect(removeSession).toHaveBeenCalledWith(SESSION);
    expect(reload).toHaveBeenCalled();
  });

  it("deletes nothing when the question is dismissed", async () => {
    const { removeSession } = draw();
    await userEvent.click(await trash());
    await userEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(removeSession).not.toHaveBeenCalled();
  });

  // The kernel refuses a conversation a pane still holds, so the delete must be
  // sequenced after the close rather than issued alongside it.
  it("closes the pane first and waits for it", async () => {
    const order: string[] = [];
    const { removeSession, onClose } = draw({
      workspaces: [
        {
          root: "/w",
          name: "w",
          sessions: [{ path: SESSION, name: "20260901-120000", title: "open here", runtimeId: "r1" }],
        } as TreeWorkspace,
      ],
    });
    onClose.mockImplementation(async () => void order.push("close"));
    removeSession.mockImplementation(async () => void order.push("remove"));

    await userEvent.click(await trash());
    await userEvent.click(confirmGo());
    expect(order).toEqual(["close", "remove"]);
    expect(onClose).toHaveBeenCalledWith(["r1"]);
  });
});
