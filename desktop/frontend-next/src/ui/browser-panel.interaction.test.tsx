// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import type { BrowserTab } from "../port/port";

// The page never draws the agent's browser: it reserves a rectangle and tells
// the shell which page to draw over it. What is held here is what it tells the
// shell, since nothing in the DOM would show a wrong answer.

const bridge = {
  shell: "electron",
  platform: "darwin",
  titleBar: false,
  showBrowserView: vi.fn(async () => {}),
  hideBrowserView: vi.fn(async () => {}),
  controlBrowserView: vi.fn(async () => {}),
  navigateBrowserView: vi.fn(async () => true),
};

const tabs: BrowserTab[] = [
  { id: "t1", target: "view-a", url: "http://127.0.0.1:5173/", title: "App", active: true },
  { id: "t2", target: "view-b", url: "https://example.com/", title: "Docs", active: false },
];

let over: Element | null = null;

async function panel(props: { tabs: BrowserTab[]; shown: boolean }) {
  vi.resetModules();
  (window as unknown as { reasonixHost: typeof bridge }).reasonixHost = bridge;
  const { BrowserPanel } = await import("./BrowserPanel");
  return render(<BrowserPanel {...props} />);
}

beforeEach(() => {
  for (const fn of [bridge.showBrowserView, bridge.hideBrowserView, bridge.controlBrowserView, bridge.navigateBrowserView]) fn.mockClear();
  over = null;
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const slot = this.classList.contains("bview");
    return { left: slot ? 40 : 0, top: slot ? 60 : 0, width: slot ? 800 : 0, height: slot ? 500 : 0 } as DOMRect;
  });
  document.elementFromPoint = () => over ?? document.querySelector(".bview");
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("which page the shell draws, and where", () => {
  it("draws the active page over the reserved rectangle, and the one picked after", async () => {
    await panel({ tabs, shown: true });
    expect(bridge.showBrowserView).toHaveBeenLastCalledWith("view-a", { x: 40, y: 60, width: 800, height: 500 });
    await userEvent.click(screen.getByRole("tab", { name: "Docs" }));
    expect(bridge.showBrowserView).toHaveBeenLastCalledWith("view-b", { x: 40, y: 60, width: 800, height: 500 });
  });

  it("puts the page away while something is drawn over its rectangle", async () => {
    await panel({ tabs, shown: true });
    bridge.hideBrowserView.mockClear();
    over = document.body;
    await act(async () => {
      document.body.appendChild(document.createElement("div"));
      await Promise.resolve();
      vi.advanceTimersByTime(20);
    });
    expect(bridge.hideBrowserView).toHaveBeenCalled();
  });

  it("draws the page once a view transition that covered its rectangle has finished", async () => {
    document.documentElement.setAttribute("data-vt", "pane");
    over = document.documentElement;
    await panel({ tabs, shown: true });
    expect(bridge.showBrowserView).not.toHaveBeenCalled();
    over = null;
    await act(async () => {
      document.documentElement.removeAttribute("data-vt");
      await Promise.resolve();
      vi.advanceTimersByTime(20);
    });
    expect(bridge.showBrowserView).toHaveBeenLastCalledWith("view-a", { x: 40, y: 60, width: 800, height: 500 });
  });

  it("draws nothing for a pane that is not on screen", async () => {
    await panel({ tabs, shown: false });
    expect(bridge.showBrowserView).not.toHaveBeenCalled();
    expect(bridge.hideBrowserView).toHaveBeenCalled();
  });

  it("puts the page away when the panel goes", async () => {
    const { unmount } = await panel({ tabs, shown: true });
    bridge.hideBrowserView.mockClear();
    unmount();
    expect(bridge.hideBrowserView).toHaveBeenCalled();
  });

  it("sends the person's controls and typed address to the page on screen", async () => {
    await panel({ tabs, shown: true });
    await userEvent.click(screen.getByRole("button", { name: "后退" }));
    expect(bridge.controlBrowserView).toHaveBeenCalledWith("view-a", "back");
    const field = screen.getByRole("textbox", { name: "网址" });
    await userEvent.clear(field);
    await userEvent.type(field, "localhost:3000{Enter}");
    expect(bridge.navigateBrowserView).toHaveBeenCalledWith("view-a", "localhost:3000");
  });
});

describe("covered", () => {
  it("answers from what the page shows at the sampled points", async () => {
    const { covered } = await import("./BrowserPanel");
    const slot = document.createElement("div");
    const inner = document.createElement("span");
    slot.appendChild(inner);
    const rect = { x: 0, y: 0, width: 100, height: 100 };
    expect(covered(slot, rect, () => slot)).toBe(false);
    expect(covered(slot, rect, () => inner)).toBe(false);
    expect(covered(slot, rect, (x, y) => (x > 90 && y > 90 ? document.body : slot))).toBe(true);
    expect(covered(slot, rect, () => null)).toBe(true);
  });
});
