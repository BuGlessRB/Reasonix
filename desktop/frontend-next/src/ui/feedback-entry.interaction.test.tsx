// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { App } from "./App";
import { dropDraft } from "./feedbackdraft";
import { MockHub } from "../port/mock_hub";

afterEach(() => {
  cleanup();
  dropDraft();
});

function hub() {
  const hub = new MockHub();
  const build = hub.portFor.bind(hub);
  hub.portFor = (rt) => {
    const port = build(rt);
    port.providerSetup = async () => null;
    port.welcomeSeen = async () => true;
    return port;
  };
  return hub;
}

describe("reaching feedback", () => {
  it("opens from the rail and closes on Escape, leaving focus on the rail entry", async () => {
    render(<App hub={hub()} />);
    const entry = await screen.findByRole("button", { name: "发送反馈" });
    await userEvent.click(entry);
    expect(await screen.findByRole("dialog", { name: "反馈" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "发送反馈", selected: true })).toBeTruthy();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "反馈" })).toBeNull();
    expect(document.activeElement).toBe(entry);
  });

  it("opens from the command palette, on the list for My feedback", async () => {
    render(<App hub={hub()} />);
    await screen.findByRole("button", { name: "发送反馈" });
    await userEvent.keyboard("{Control>}k{/Control}");
    await userEvent.keyboard("feedback");
    await userEvent.click(await screen.findByText("我的反馈"));
    expect(await screen.findByRole("tab", { name: "我的反馈", selected: true })).toBeTruthy();
    expect(await screen.findByText("FB-7K3M-9QX2")).toBeTruthy();
  });
});
