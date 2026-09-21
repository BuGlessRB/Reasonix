// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SayCard } from "./SayCard";

afterEach(cleanup);

describe("completed reasoning", () => {
  it("starts folded and opens on click", async () => {
    const { container } = render(<SayCard item={{ t: "say", id: "s", text: "answer", reasoning: "reason", done: true }} />);
    const details = container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    await userEvent.click(screen.getByText(/思考/));
    expect(details.open).toBe(true);
  });
});
