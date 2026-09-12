// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Policy } from "./Policy";
import { MockPort } from "../port/mock";
import type { AgentPort, ApprovalMode, Preset, SessionStatus } from "../port/port";

afterEach(cleanup);

const status = (preset: Preset, effort: string | undefined, mode: ApprovalMode) =>
  ({ preset, effort, toolApprovalMode: mode } as SessionStatus);

function draw(over: {
  status?: SessionStatus | null;
  efforts?: string[];
  port?: Partial<AgentPort>;
} = {}) {
  const port = { ...(new MockPort() as unknown as AgentPort), ...over.port };
  const onChanged = vi.fn();
  const r = render(
    <Policy
      port={port}
      status={over.status === undefined ? status("balanced", "auto", "ask") : over.status}
      efforts={over.efforts ?? ["auto", "low", "medium", "high"]}
      onChanged={onChanged}
    />,
  );
  // The trigger by its own element: getByRole would move to the menu's
  // buttons the moment it is open, which is exactly when this is asked.
  return { ...r, port, onChanged, shelf: () => r.container.querySelector(".policy .vl")?.textContent };
}

describe("what the closed control says", () => {
  it("keeps a quiet way into policy for a baseline session", () => {
    const { container } = draw();
    expect(container.querySelector(".policy[data-quiet]")).toBeTruthy();
    expect(container.querySelector(".policy > button")?.textContent).toBe("执行");
  });

  it("carries the whole reading on hover, so nothing is actually lost", () => {
    const { container } = draw({ status: status("delivery", "auto", "ask") });
    expect(container.querySelector("button")?.getAttribute("title")).toBe("交付 · auto · 询问");
  });

  it("draws nothing at all before the kernel has answered", () => {
    const { container } = draw({ status: null });
    expect(container.querySelector(".policy")).toBeNull();
    expect(container.textContent).toBe("");
  });

  it("tones a fully-permitted session and marks it, without repainting the control", () => {
    const { container } = draw({ status: status("balanced", "auto", "yolo") });
    const risk = container.querySelector(".vl .polrisk");
    expect(risk?.textContent).toContain("全部放行");
    expect(container.querySelector(".vl .polwarn svg")).not.toBeNull();
    // The button itself stays quiet; only the segment that must be remembered
    // is toned, or every ordinary turn is watched too.
    expect(container.querySelector("button")?.className).not.toMatch(/risk|danger/);
  });

  it("leaves a stricter-than-default posture visible but untoned", () => {
    const { container } = draw({ status: status("balanced", "auto", "dontAsk") });
    expect(container.querySelector(".vl")?.textContent).toBe("不询问");
    expect(container.querySelector(".vl .polrisk")).toBeNull();
  });
});

describe("the shelf reads committed fact, never the answer being waited on", () => {
  // Sabotage C. A summary that moves on click says the session is running under
  // something the kernel has not accepted — and may refuse.
  it("does not adopt a rung the kernel has not answered for", async () => {
    let release!: () => void;
    const setEffort = vi.fn(() => new Promise<void>((ok) => { release = ok; }));
    const { shelf } = draw({ status: status("delivery", "auto", "ask"), port: { setEffort } });
    await userEvent.click(screen.getByRole("button", { name: "交付" }));
    await userEvent.click(screen.getByRole("button", { name: "high" }));
    expect(setEffort).toHaveBeenCalledWith("high");
    // Asked for, not yet true.
    expect(shelf()).toBe("交付");
    release();
    await waitFor(() => expect(shelf()).toBe("交付"));
  });

  it("moves only once the kernel's own reading says so", async () => {
    const { rerender, shelf } = draw({ status: status("delivery", "auto", "ask") });
    expect(shelf()).toBe("交付");
    rerender(
      <Policy port={new MockPort() as unknown as AgentPort} status={status("delivery", "high", "ask")}
        efforts={["auto", "high"]} onChanged={() => {}} />,
    );
    expect(shelf()).toBe("交付 · High");
  });

  it("leaves the summary alone when the kernel refuses", async () => {
    const setApprovalMode = vi.fn(() => Promise.reject(new Error("not while a turn is running")));
    const { shelf, container } = draw({ status: status("delivery", "auto", "ask"), port: { setApprovalMode } });
    await userEvent.click(screen.getByRole("button", { name: "交付" }));
    await userEvent.click(screen.getByRole("button", { name: "全部放行" }));
    await waitFor(() => expect(container.querySelector(".segbad")).toBeTruthy());
    expect(shelf()).toBe("交付");
  });
});

describe("a model that publishes no rungs", () => {
  // That the open menu draws no ladder is policy.interaction's; what is this
  // file's is the other half — a stale rung may not leak onto the shelf, while
  // preset and permissions remain reachable.
  it("claims no stale rung but keeps the other policy fields reachable", () => {
    const { container } = draw({ status: status("balanced", "high", "ask"), efforts: [] });
    expect(container.querySelector(".policy[data-quiet]")).toBeTruthy();
    expect(container.querySelector(".policy .vl")).toBeNull();
    expect(container.querySelector(".policy > button")?.getAttribute("title")).toBe("均衡 · 询问");
  });
});
