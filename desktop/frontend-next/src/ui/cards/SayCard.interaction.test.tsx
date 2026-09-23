// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SayCard } from "./SayCard";
import { FOLD_DEFAULTS, setFoldModes } from "../../state/prefs";

afterEach(() => {
  cleanup();
  setFoldModes(FOLD_DEFAULTS);
});

describe("completed reasoning", () => {
  it("starts folded and opens on click", async () => {
    const { container } = render(<SayCard item={{ t: "say", id: "s", text: "answer", reasoning: "reason", done: true }} />);
    const details = container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    await userEvent.click(screen.getByText(/思考/));
    expect(details.open).toBe(true);
  });
});

describe("the folding preference", () => {
  const item = { t: "say" as const, id: "s", text: "answer", reasoning: "reason", done: true };
  const details = (el: HTMLElement) => el.querySelector("details") as HTMLDetailsElement;

  it("opens thinking by default once it is set to open", () => {
    setFoldModes({ thinking: "open" });
    expect(details(render(<SayCard item={item} />).container).open).toBe(true);
  });

  it("moves an untouched block and leaves a clicked one alone", async () => {
    const a = details(render(<SayCard item={item} />).container);
    const b = details(render(<SayCard item={{ ...item, id: "t" }} />).container);
    await userEvent.click(b.querySelector("summary") as HTMLElement);
    await userEvent.click(b.querySelector("summary") as HTMLElement);
    expect(b.open).toBe(false);
    act(() => setFoldModes({ thinking: "open" }));
    expect(a.open).toBe(true);
    expect(b.open).toBe(false);
  });

  it("under live, opens while thinking streams and folds once the answer starts", () => {
    window.matchMedia ??= (() => ({ matches: true, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
    setFoldModes({ thinking: "live" });
    const streaming = { t: "say" as const, id: "l", text: "", reasoning: "reason", done: false };
    const { container, rerender } = render(<SayCard item={streaming} />);
    expect(details(container).open).toBe(true);
    rerender(<SayCard item={{ ...streaming, text: "answer" }} />);
    expect(details(container).open).toBe(false);
  });
});
