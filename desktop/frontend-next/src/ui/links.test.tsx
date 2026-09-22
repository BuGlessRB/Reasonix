// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import "./testkit";
import { useLinkRouting } from "./links";
import * as hostModule from "../port/host";
import type { AgentPort, BrowserTab } from "../port/port";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function drawsViews(yes: boolean) {
  const real = hostModule.host();
  vi.spyOn(hostModule, "host").mockReturnValue({ ...real, drawsBrowserViews: () => yes } as ReturnType<typeof hostModule.host>);
}

function stage(yes: boolean) {
  drawsViews(yes);
  const openExternal = vi.fn(async () => {});
  const browserOpen = vi.fn(async () => ({ target: "t1", url: "x" }) as BrowserTab);
  const reveal = vi.fn();
  const port = { openExternal, browserOpen } as unknown as AgentPort;
  function Harness() {
    useLinkRouting(port, reveal, vi.fn());
    return <a href="https://example.com/a">go</a>;
  }
  const view = render(<Harness />);
  const link = view.container.querySelector("a") as HTMLAnchorElement;
  return { link, openExternal, browserOpen, reveal };
}

// A page opened outside is a page nobody in the conversation can see — not the
// person looking at the window, and not the model that would have to read it.
describe("where a link in this window goes", () => {
  it("opens in the browser this window draws, and shows it", async () => {
    const { link, browserOpen, openExternal, reveal } = stage(true);
    link.click();
    expect(browserOpen).toHaveBeenCalledWith("https://example.com/a", true);
    expect(reveal).toHaveBeenCalled();
    expect(openExternal).not.toHaveBeenCalled();
  });

  it("leaves for the machine's own browser on a modifier, which is the way out", () => {
    const { link, browserOpen, openExternal } = stage(true);
    link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, ctrlKey: true }));
    expect(openExternal).toHaveBeenCalledWith("https://example.com/a");
    expect(browserOpen).not.toHaveBeenCalled();
  });

  it("has only that way where the shell draws no views", () => {
    const { link, browserOpen, openExternal } = stage(false);
    link.click();
    expect(openExternal).toHaveBeenCalledWith("https://example.com/a");
    expect(browserOpen).not.toHaveBeenCalled();
  });

  it("leaves anything that is not an http address to whatever else handles it", () => {
    drawsViews(true);
    const openExternal = vi.fn(async () => {});
    const port = { openExternal, browserOpen: vi.fn() } as unknown as AgentPort;
    function Harness() {
      useLinkRouting(port, vi.fn(), vi.fn());
      return <a href="#anchor">go</a>;
    }
    const view = render(<Harness />);
    const link = view.container.querySelector("a") as HTMLAnchorElement;
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });
    link.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(openExternal).not.toHaveBeenCalled();
  });
});
