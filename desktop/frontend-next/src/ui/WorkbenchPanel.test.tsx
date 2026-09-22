// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MockPort } from "../port/mock";
import { WorkbenchPanel } from "./WorkbenchPanel";

afterEach(cleanup);

describe("WorkbenchPanel", () => {
  it("opens workspace files as tabs and saves an edited buffer", async () => {
    const user = userEvent.setup();
    const port = new MockPort();
    const save = vi.spyOn(port, "saveWorkspaceFile");
    render(<WorkbenchPanel port={port} tabs={[]} manual={false} shown scheme="light" changes={[]} onCloseManual={vi.fn()} onSurfaces={vi.fn()} onExternal={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "README.md" }));
    expect(screen.getByRole("tab", { name: "README.md" }).getAttribute("aria-selected")).toBe("true");
    await user.click(screen.getByRole("button", { name: "编辑" }));
    const editor = await screen.findByRole("textbox", { name: "文件内容" });
    await user.type(editor, "updated");
    await user.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(save).toHaveBeenCalled());
  });

  it("puts manual and agent browsers on the same tab strip", () => {
    render(<WorkbenchPanel port={new MockPort()} tabs={[{ id: "b1", target: "t1", url: "https://example.com", title: "Agent page", active: true }]} manual shown={false} scheme="dark" changes={[]} onCloseManual={vi.fn()} onSurfaces={vi.fn()} onExternal={vi.fn()} />);
    expect(screen.getByRole("tab", { name: "浏览器" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Agent page" })).toBeTruthy();
  });
});
