// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { MarketGroup } from "./Market";
import { MockPort } from "../port/mock";
import { HttpError } from "../port/port";
import type { AgentPort } from "../port/port";

afterEach(cleanup);

const signedIn = { signedIn: true, user: { handle: "demo", email: "demo@example.com", label: "demo" } };
const pinned = "https://github.com/demo/themes/tree/" + "a".repeat(40) + "/dusk";

async function openMine(port: AgentPort, onInstalled = () => {}) {
  render(<MarketGroup port={port} onInstalled={onInstalled} account={signedIn} onSignIn={() => {}} />);
  await userEvent.click(screen.getByRole("radio", { name: "我的发布" }));
}

describe("the account's own packages", () => {
  it("saves a private package out of review and shows it as private", async () => {
    const port = new MockPort() as unknown as AgentPort;
    const publish = vi.spyOn(port, "publishMarket");
    render(<MarketGroup port={port} onInstalled={() => {}} account={signedIn} onSignIn={() => {}} />);
    await userEvent.click(screen.getByRole("radio", { name: "发布" }));
    await userEvent.click(screen.getByRole("radio", { name: "主题" }));
    fireEvent.change(document.querySelector('[data-value="name"]')!, { target: { value: "dusk" } });
    fireEvent.change(document.querySelector('[data-value="source"]')!, { target: { value: pinned } });
    await userEvent.click(screen.getByRole("checkbox", { name: /仅自己可见/ }));
    await userEvent.click(screen.getByRole("button", { name: "保存为私有" }));

    await waitFor(() => expect(publish).toHaveBeenCalled());
    expect(publish.mock.calls[0]![0]).toMatchObject({ name: "dusk", visibility: "private" });
    expect(await screen.findByText("已保存 demo/dusk 0.1.0，仅自己可见")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "查看我的发布" }));
    const row = (await screen.findByText("dusk")).closest("li")!;
    expect(row.textContent).toContain("私有");
    expect(row.textContent).toContain("提交审核");
  });

  it("sends a private package to review only on request", async () => {
    const port = new MockPort() as unknown as AgentPort;
    const submit = vi.spyOn(port, "submitMarket");
    await openMine(port);
    const row = (await screen.findByText("night-desk")).closest("li")!;
    expect(row.textContent).toContain("私有");
    expect(submit).not.toHaveBeenCalled();
    await userEvent.click(row.querySelector<HTMLButtonElement>('[data-action="market.submit"]')!);
    await waitFor(() => expect(submit).toHaveBeenCalledWith("demo/night-desk"));
    await waitFor(() => expect(row.textContent).toContain("审核中"));
    expect(row.querySelector('[data-action="market.submit"]')).toBeNull();
  });

  it("installs an unreviewed package against the digest its preview showed", async () => {
    const port = new MockPort() as unknown as AgentPort;
    const plan = vi.spyOn(port, "planOwnMarket");
    const install = vi.spyOn(port, "installOwnMarket");
    const onInstalled = vi.fn();
    await openMine(port, onInstalled);
    const row = (await screen.findByText("ship-notes")).closest("li")!;
    await userEvent.click(row.querySelector<HTMLButtonElement>('[data-action="market.own-inspect"]')!);

    expect(await screen.findByText("未审核 · 仅你可见")).toBeTruthy();
    expect(plan).toHaveBeenCalledWith({ slug: "demo/ship-notes", replace: false });
    expect(install).not.toHaveBeenCalled();
    const shown = await plan.mock.results[0]!.value;
    await userEvent.click(screen.getByRole("button", { name: "安装" }));

    await waitFor(() => expect(install).toHaveBeenCalled());
    expect(install.mock.calls[0]![0]).toEqual({
      slug: "demo/ship-notes", version: shown.version, planId: shown.planId, replace: false, digest: shown.contentDigest,
    });
    await waitFor(() => expect(onInstalled).toHaveBeenCalled());
  });

  it("offers nothing to install when the kernel cannot pin the preview", async () => {
    const port = new MockPort() as unknown as AgentPort;
    vi.spyOn(port, "planOwnMarket").mockRejectedValue(
      new HttpError(409, "not pinnable", { code: "market.not_pinnable", error: "not pinnable" }),
    );
    const install = vi.spyOn(port, "installOwnMarket");
    await openMine(port);
    const row = (await screen.findByText("lint-kit")).closest("li")!;
    await userEvent.click(row.querySelector<HTMLButtonElement>('[data-action="market.own-inspect"]')!);
    expect(await screen.findByText(/该来源的内容无法固定/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "安装" })).toBeNull();
    expect(install).not.toHaveBeenCalled();
  });
});
