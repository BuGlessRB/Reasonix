// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Market } from "./Market";
import { MockPort } from "../port/mock";
import type { AgentPort, MarketPackage, MarketQuery } from "../port/port";

afterEach(cleanup);

const row = (slug: string, pinned?: boolean): MarketPackage => ({
  kind: "skill", handle: slug.split("/")[0], name: slug.split("/")[1], slug, summary: "s", description: "", homepage: "",
  repoUrl: "", tags: [], latestVersion: "1.0.0", installCount: 3, starCount: 0, verified: false, status: "active", updatedAt: "", pinned,
});

describe("market list", () => {
  it("asks the registry for installable packages instead of filtering a page", async () => {
    const port = new MockPort() as unknown as AgentPort;
    const asked: MarketQuery[] = [];
    port.marketList = vi.fn(async (q: MarketQuery) => {
      asked.push(q);
      return { packages: q.pinned ? [row("a/kit", true)] : [row("a/kit", true), row("b/raw", false), row("c/old")], limit: 24, offset: 0 };
    });
    render(<Market port={port} onInstalled={() => {}} />);

    await screen.findByText("raw");
    expect(screen.getByText("可安装")).toBeTruthy();
    expect(screen.getByText("未固定")).toBeTruthy();
    // A registry that did not say is not shown as either.
    expect(document.querySelectorAll(".mkt-pinned")).toHaveLength(2);

    await userEvent.click(screen.getByRole("checkbox", { name: "只看可安装" }));
    await waitFor(() => expect(screen.queryByText("raw")).toBeNull());
    expect(asked.at(-1)).toMatchObject({ pinned: true, offset: 0 });
  });

  it("opens an entry from the keyboard", async () => {
    const port = new MockPort() as unknown as AgentPort;
    port.marketList = async () => ({ packages: [row("a/kit", true)], limit: 24, offset: 0 });
    port.marketDetail = vi.fn(async () => ({ package: row("a/kit", true), pinned: true }));
    render(<Market port={port} onInstalled={() => {}} />);

    (await screen.findByRole("button", { name: /kit/ })).focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(port.marketDetail).toHaveBeenCalledWith("a/kit"));
  });
});
