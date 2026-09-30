// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { MyPackages } from "./MarketPublish";
import { MockPort } from "../port/mock";
import type { AgentPort, MarketPackage, MarketPlan } from "../port/port";

afterEach(cleanup);

describe("my packages connection lifetime", () => {
  it("refreshes the parent's inventory after a blocked Back during installation", async () => {
    const port = new MockPort() as unknown as AgentPort;
    const pkg = (await port.myMarket()).find((p) => p.slug === "demo/ship-notes")!;
    const plan = await port.planOwnMarket({ slug: pkg.slug });
    const read = vi.spyOn(port, "myMarket").mockResolvedValueOnce([pkg])
      .mockResolvedValueOnce([{ ...pkg, installed: { version: pkg.latestVersion, contentHash: "installed-digest" } }]);
    let finish!: (plan: MarketPlan) => void;
    vi.spyOn(port, "installOwnMarket").mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const onInstalled = vi.fn();
    render(<MyPackages port={port} onInstalled={onInstalled} />);
    await userEvent.click(await screen.findByRole("button", { name: "安装" }));
    await userEvent.click(await screen.findByRole("button", { name: "安装" }));
    await userEvent.click(screen.getByRole("button", { name: "返回" }));
    await act(async () => finish({ ...plan, applied: true, status: "done", actions: plan.actions?.map((a) => ({ ...a, status: "done" })) }));
    expect(onInstalled).toHaveBeenCalledTimes(1);
    expect(read).toHaveBeenCalledTimes(2);
    await userEvent.click(screen.getByRole("button", { name: "返回我的发布" }));
    expect(await screen.findByText("已安装")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "安装" })).toBeNull();
  });

  it("closes the old account's package before the new connection can preview it", async () => {
    const port = new MockPort() as unknown as AgentPort;
    const next = new MockPort() as unknown as AgentPort;
    const pkg = (await port.myMarket()).find((p) => p.slug === "demo/ship-notes")!;
    vi.spyOn(port, "myMarket").mockResolvedValue([pkg]);
    vi.spyOn(next, "myMarket").mockResolvedValue([{ ...pkg, slug: "other/new-package", name: "new-package" }]);
    const preview = vi.spyOn(next, "planOwnMarket");
    const install = vi.spyOn(next, "installOwnMarket");
    const view = render(<MyPackages port={port} onInstalled={() => {}} />);
    await userEvent.click(await screen.findByRole("button", { name: "安装" }));
    await screen.findByText(/demo\/ship-notes .* 将安装以下内容/);
    view.rerender(<MyPackages port={next} onInstalled={() => {}} />);
    await screen.findByText("new-package");
    expect(screen.queryByText(/demo\/ship-notes .* 将安装以下内容/)).toBeNull();
    expect(preview).not.toHaveBeenCalled();
    expect(install).not.toHaveBeenCalled();
  });

  it.each(["success", "failure"])("isolates an old submit %s from a new submission with the same slug", async (outcome) => {
    const port = new MockPort() as unknown as AgentPort;
    const next = new MockPort() as unknown as AgentPort;
    const pkg = { ...(await port.myMarket()).find((p) => p.slug === "demo/ship-notes")!, status: "private" };
    vi.spyOn(port, "myMarket").mockResolvedValue([pkg]);
    vi.spyOn(next, "myMarket").mockResolvedValue([pkg]);
    let finishOld!: (pkg: MarketPackage) => void;
    let failOld!: (error: Error) => void;
    let finishNew!: (pkg: MarketPackage) => void;
    vi.spyOn(port, "submitMarket").mockImplementation(() => new Promise((resolve, reject) => { finishOld = resolve; failOld = reject; }));
    const submit = vi.spyOn(next, "submitMarket").mockImplementation(() => new Promise((resolve) => { finishNew = resolve; }));
    const view = render(<MyPackages port={port} onInstalled={() => {}} />);
    await userEvent.click(await screen.findByRole("button", { name: "提交审核" }));
    view.rerender(<MyPackages port={next} onInstalled={() => {}} />);
    const send = await screen.findByRole<HTMLButtonElement>("button", { name: "提交审核" });
    expect(send.disabled).toBe(false);
    await userEvent.click(send);
    await act(async () => {
      if (outcome === "success") finishOld({ ...pkg, name: "old-response", status: "pending" });
      else failOld(new Error("old submission failed"));
    });
    expect(screen.queryByText("old-response")).toBeNull();
    expect(screen.queryByText("old submission failed")).toBeNull();
    expect(screen.getByText("ship-notes")).toBeTruthy();
    const sending = screen.getByRole<HTMLButtonElement>("button", { name: "提交中…" });
    expect(sending.disabled).toBe(true);
    await userEvent.click(sending);
    expect(submit).toHaveBeenCalledTimes(1);
    await act(async () => finishNew({ ...pkg, status: "pending" }));
    expect(screen.getByText("审核中")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "提交中…" })).toBeNull();
  });
});
