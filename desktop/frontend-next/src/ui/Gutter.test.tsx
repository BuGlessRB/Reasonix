// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { DOCK, Gutter, dockMax } from "./Gutter";

afterEach(cleanup);

describe("workbench width on wide displays", () => {
  it("keeps the old limit on ordinary windows and leaves half a wide window for the other columns", () => {
    expect(dockMax(1600)).toBe(880);
    expect(dockMax(3440)).toBe(1600);
  });

  it("lets the keyboard widen the workbench past 880 only when there is room", () => {
    let width = 880;
    const view = render(
      <Gutter edge="r" span={DOCK} width={width} max={dockMax(3440)} label="Workbench width"
        open onWidth={(next) => { width = next; }} onOpen={() => {}} />,
    );
    const separator = view.getByRole("separator");
    fireEvent.keyDown(separator, { key: "ArrowLeft", shiftKey: true });
    expect(width).toBe(928);
    expect(separator.getAttribute("aria-valuemax")).toBe("1600");

    view.rerender(
      <Gutter edge="r" span={DOCK} width={880} max={dockMax(1600)} label="Workbench width"
        open onWidth={(next) => { width = next; }} onOpen={() => {}} />,
    );
    fireEvent.keyDown(separator, { key: "ArrowLeft", shiftKey: true });
    expect(width).toBe(928);
    expect(separator.getAttribute("aria-valuemax")).toBe("880");
  });
});
