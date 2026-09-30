// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Packages } from "./Packages";
import { MockPort } from "../port/mock";
import type { AgentPort, PluginExport, PluginPlan } from "../port/port";

afterEach(cleanup);

describe("installed package operations", () => {
  it.each(["remove", "export"])("blocks conflicting row actions during %s and recovers after failure", async (operation) => {
    const port = new MockPort() as unknown as AgentPort;
    const packages = await port.plugins();
    let fail!: (error: Error) => void;
    const remove = vi.spyOn(port, "removePlugin");
    const exportPackage = vi.spyOn(port, "exportPlugin");
    if (operation === "remove") {
      remove.mockImplementationOnce(() => new Promise<PluginPlan>((_, reject) => { fail = reject; }));
    } else {
      exportPackage.mockImplementationOnce(() => new Promise<PluginExport>((_, reject) => { fail = reject; }));
    }
    const toggle = vi.spyOn(port, "setPluginEnabled");
    const onUpdate = vi.fn();
    const onChanged = vi.fn();
    render(<Packages port={port} packages={packages} onChanged={onChanged} updating="" onUpdate={onUpdate} />);
    const row = within(document.querySelector('[data-extension-name="review-kit"]') as HTMLElement);
    await userEvent.click(row.getByRole("button", { name: "移除 review-kit" }));
    if (operation === "remove") {
      await userEvent.click(row.getByRole("button", { name: "删除" }));
    } else {
      await userEvent.click(row.getByRole("button", { name: "导出" }));
    }

    const enable = row.getByRole<HTMLButtonElement>("switch", { name: "关闭 review-kit" });
    await userEvent.click(enable);
    await userEvent.click(row.getByRole("button", { name: "更新" }));
    expect(toggle).not.toHaveBeenCalled();
    expect(onUpdate).not.toHaveBeenCalled();
    expect(enable.disabled).toBe(true);
    expect(row.getByRole<HTMLButtonElement>("button", { name: "移除 review-kit" }).disabled).toBe(true);
    expect(row.getByRole<HTMLButtonElement>("button", { name: "取消" }).disabled).toBe(true);
    if (operation === "remove") {
      expect(row.getByRole<HTMLButtonElement>("button", { name: "导出" }).disabled).toBe(true);
    } else {
      await userEvent.click(row.getByRole("button", { name: "删除" }));
      expect(remove).not.toHaveBeenCalled();
    }
    expect(screen.getByRole<HTMLButtonElement>("switch", { name: "启用 notion-bridge" }).disabled).toBe(false);

    await act(async () => fail(new Error(`${operation} unavailable`)));
    expect(await row.findByText(`${operation} unavailable`)).toBeTruthy();
    expect(enable.disabled).toBe(false);
    expect(row.getByRole<HTMLButtonElement>("button", { name: "更新" }).disabled).toBe(false);
    expect(row.getByRole<HTMLButtonElement>("button", { name: "导出" }).disabled).toBe(false);
    expect(onChanged).toHaveBeenCalledTimes(1);
  });
});
