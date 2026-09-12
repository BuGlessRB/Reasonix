// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "../testkit";
import { Context } from "./Context";
import { MockPort } from "../../port/mock";
import type { AgentPort, ContextBreakdown } from "../../port/port";

afterEach(cleanup);

// The user's own shape: a 1M window that folds at 160k because of a bound they
// never set. The fold point is 16% of the window, and before this the only way
// to move it was a settings sheet two clicks away.
const wide: ContextBreakdown = {
  used: 82_000, window: 1_000_000, compact_at: 160_000,
  boundary: "economic", capacity_at: 850_000,
  system: 20_000, tools: 12_000, user: 10_000, reply: 20_000, output: 20_000,
};

const mount = async (over: Partial<ContextBreakdown> = {}) => {
  const port = new MockPort() as unknown as AgentPort;
  await port.setContextWindow(1_000_000);
  const onCtx = vi.fn();
  render(<Context ctx={{ ...wide, ...over }} legend port={port} onCtx={onCtx} />);
  return { port, onCtx };
};

describe("changing the fold point where it is read", () => {
  it("offers the fold point as the way in, not only as a figure", async () => {
    await mount();
    const entry = screen.getByRole("button", { name: "160k" });
    expect(entry.getAttribute("aria-expanded")).toBe("false");
  });

  it("opens the same three choices the settings sheet offers", async () => {
    await mount();
    await userEvent.click(screen.getByRole("button", { name: "160k" }));
    await screen.findByRole("group", { name: "维护点" });
    for (const label of ["默认", "自定义", "按容量"]) {
      expect(screen.getByRole("button", { name: label })).toBeTruthy();
    }
  });

  // The footnote answers "why 160k"; the chosen mode answers "what this does".
  // Stacked, they are two grey paragraphs in one narrow column.
  it("stands the footnote down while the editor is open", async () => {
    await mount();
    expect(screen.getByText(/不随窗口放大/)).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "160k" }));
    await screen.findByRole("group", { name: "维护点" });
    expect(screen.queryByText(/不随窗口放大/)).toBeNull();
  });

  // The write rebuilds the runtime, so the gauge that comes back is the kernel's
  // — not this reply's arithmetic. A panel that kept its old figure would show a
  // fold point the session no longer folds at.
  it("hands back the gauge the kernel rebuilt, not a computed one", async () => {
    const { onCtx } = await mount();
    await userEvent.click(screen.getByRole("button", { name: "160k" }));
    await userEvent.click(await screen.findByRole("button", { name: "按容量" }));
    await waitFor(() => expect(onCtx).toHaveBeenCalled());
    expect(onCtx.mock.calls.at(-1)?.[0].compact_at).toBe(850_000);
  });

  // Two editors in one column, each answering for a different ceiling.
  it("closes the window editor when the fold editor opens", async () => {
    await mount();
    await userEvent.click(screen.getByRole("button", { name: "1.0M" }));
    expect(screen.getByLabelText("上下文窗口（tokens）")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "160k" }));
    await screen.findByRole("group", { name: "维护点" });
    expect(screen.queryByLabelText("上下文窗口（tokens）")).toBeNull();
  });
});
