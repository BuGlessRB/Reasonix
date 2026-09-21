// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import "./testkit";
import { PaneNav } from "./PaneNav";

afterEach(cleanup);

const draw = (rows = 3, pages = 0) => {
  const onPick = vi.fn();
  render(<PaneNav view="flow" onPick={onPick} rows={rows} pages={pages} dock={false} onDock={() => {}} />);
  return onPick;
};

describe("pane navigation", () => {
  it("keeps only conversation and the readable run analysis", () => {
    draw();
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["对话", "运行分析"]);
    expect(screen.queryByText("任务")).toBeNull();
    expect(screen.queryByText("运行详情")).toBeNull();
  });

  it("opens analysis directly and adds the workbench when it has a surface", () => {
    const onPick = draw(2, 1);
    fireEvent.click(screen.getByRole("tab", { name: "运行分析" }));
    expect(onPick).toHaveBeenCalledWith("analysis");
    expect(screen.getByRole("tab", { name: /工作台/ })).toBeTruthy();
  });
});
