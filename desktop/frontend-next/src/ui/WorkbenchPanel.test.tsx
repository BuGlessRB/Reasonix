// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { EditorView } from "@codemirror/view";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MockPort } from "../port/mock";
import { WorkbenchPanel } from "./WorkbenchPanel";

afterEach(cleanup);

describe("WorkbenchPanel", () => {
  it("opens workspace files as tabs and saves what was typed in the editor", async () => {
    const user = userEvent.setup();
    const port = new MockPort();
    const save = vi.spyOn(port, "saveWorkspaceFile");
    render(<WorkbenchPanel port={port} tabs={[]} manual={false} shown scheme="light" changes={[]} onCloseManual={vi.fn()} onSurfaces={vi.fn()} onExternal={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "README.md" }));
    expect(screen.getByRole("tab", { name: "README.md" }).getAttribute("aria-selected")).toBe("true");
    // A document opens rendered; editing is the next mode over.
    await waitFor(() => expect(document.querySelector(".workbench-read .md")).toBeTruthy());
    expect(screen.getByRole("button", { name: "阅读" }).getAttribute("aria-pressed")).toBe("true");
    await user.click(screen.getByRole("button", { name: "编辑" }));
    const content = await waitFor(() => {
      const el = document.querySelector(".cm-content");
      if (!el) throw new Error("editor not mounted");
      return el as HTMLElement;
    });
    const view = EditorView.findFromDOM(content)!;
    act(() => view.dispatch({ changes: { from: view.state.doc.length, insert: "updated" } }));
    await user.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0][0].content.endsWith("updated")).toBe(true);
    // What reading shows is the draft, so an edit is visible before it is saved.
    act(() => view.dispatch({ changes: { from: view.state.doc.length, insert: "\n\n## Draft heading" } }));
    await user.click(screen.getByRole("button", { name: "阅读" }));
    await waitFor(() => expect(document.querySelector(".workbench-read h2")?.textContent).toBe("Draft heading"));
  });

  it("shows the start page only while the agent has none, and gives way to the agent's", () => {
    const props = { port: new MockPort(), manual: true, shown: false, scheme: "dark" as const, changes: [], onCloseManual: vi.fn(), onSurfaces: vi.fn(), onExternal: vi.fn() };
    const { rerender } = render(<WorkbenchPanel {...props} tabs={[]} />);
    expect(screen.getByRole("tab", { name: "浏览器" })).toBeTruthy();
    rerender(<WorkbenchPanel {...props} tabs={[{ id: "b1", target: "t1", url: "https://top.baidu.com", title: "百度热搜", active: true }]} />);
    expect(screen.queryByRole("tab", { name: "浏览器" })).toBeNull();
    expect(screen.getByRole("tab", { name: "百度热搜" }).getAttribute("aria-selected")).toBe("true");
  });

  it("keeps the column while another tab is open, and folds it with the last", async () => {
    const user = userEvent.setup();
    const onCloseManual = vi.fn();
    const pages = [
      { id: "b1", target: "t1", url: "https://top.baidu.com", title: "Agent page", active: true },
      { id: "b2", target: "t2", url: "https://example.com", title: "My page", active: false },
    ];
    render(<WorkbenchPanel port={new MockPort()} tabs={pages} manual shown={false} scheme="dark" changes={[]} onCloseManual={onCloseManual} onSurfaces={vi.fn()} onExternal={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "关闭 Agent page" }));
    expect(onCloseManual).not.toHaveBeenCalled();
    expect(screen.getByRole("tab", { name: "My page" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "关闭 My page" }));
    expect(onCloseManual).toHaveBeenCalledTimes(1);
  });

  it("shows a page the person opened beside the agent's, while the agent's tab stays active", () => {
    const props = { port: new MockPort(), manual: true, shown: false, scheme: "dark" as const, changes: [], onCloseManual: vi.fn(), onSurfaces: vi.fn(), onExternal: vi.fn() };
    const agent = { id: "b1", target: "t1", url: "https://top.baidu.com", title: "百度热搜", active: true };
    const { rerender } = render(<WorkbenchPanel {...props} tabs={[agent]} />);
    rerender(<WorkbenchPanel {...props} tabs={[agent, { id: "b2", target: "t2", url: "https://example.com", title: "Link", active: false }]} />);
    expect(screen.getByRole("tab", { name: "Link" }).getAttribute("aria-selected")).toBe("true");
  });

  it("shows the file that was picked from the list rather than leaving the list over it", async () => {
    const user = userEvent.setup();
    const { container } = render(<WorkbenchPanel port={new MockPort()} tabs={[]} manual={false} shown scheme="light" changes={[]} onCloseManual={vi.fn()} onSurfaces={vi.fn()} onExternal={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "文件" }));
    expect(container.querySelector(".workbench-body")?.hasAttribute("data-files")).toBe(true);
    await user.click(await screen.findByRole("button", { name: "README.md" }));
    expect(container.querySelector(".workbench-body")?.hasAttribute("data-files")).toBe(false);
    expect(screen.getByRole("tab", { name: "README.md" }).getAttribute("aria-selected")).toBe("true");
  });
});
