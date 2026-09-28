// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import "./testkit";
import { MockPort } from "../port/mock";
import { Pane, type PaneReport } from "./Pane";

afterEach(cleanup);

it("shows a turn already running when another device joins the pane", async () => {
  const port = new MockPort();
  const snapshot = await port.status();
  let kernelRunning = true;
  vi.spyOn(port, "status").mockImplementation(async () => ({ ...snapshot, running: kernelRunning }));
  const onReport = vi.fn<(id: string, report: PaneReport) => void>();

  render(
    <Pane
      port={port}
      rt={{ id: "r1", base: "/rt/r1", root: "/workspace", name: "workspace", sessionPath: "/sessions/active.jsonl" }}
      title="active"
      active
      visible
      sideHost={null}
      side={false}
      onFocus={() => {}}
      onReport={onReport}
      onSessionChanged={() => {}}
      pulse={0}
      findPulse={0}
      onSettings={() => {}}
      needsProject={false}
      onOpenProject={() => {}}
      onKeepHere={() => {}}
      theme="light"
      dockW={320}
      dockMax={640}
      onDockW={() => {}}
    />,
  );

  await waitFor(() => expect(onReport).toHaveBeenCalledWith("r1", expect.objectContaining({ live: true, run: "running" })));
  onReport.mockClear();
  kernelRunning = false;
  await waitFor(() => expect(onReport).toHaveBeenCalledWith("r1", expect.objectContaining({ live: false, run: "idle" })));
});
