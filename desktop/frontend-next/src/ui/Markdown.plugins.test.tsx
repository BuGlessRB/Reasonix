// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { Markdown } from "./Markdown";

afterEach(cleanup);

// The deferred plugins are functions, and useState takes a function argument as
// a lazy initializer. Storing what the attacher returns hands unified a
// transformer to attach, which then runs with no tree: every card mounted after
// the first listing threw and fell back to its own source text.
it("renders markdown on a card mounted after the highlighter arrived", async () => {
  const listing = render(<Markdown text={"```go\nfunc main() {}\n```"} />);
  await waitFor(() => expect(listing.container.querySelector("code.hljs")).toBeTruthy());
  cleanup();

  const { container } = render(<Markdown text={"**粗体**与 `行内`"} />);
  expect(container.querySelector("strong")).toBeTruthy();
  expect(container.querySelector("code")).toBeTruthy();
});
