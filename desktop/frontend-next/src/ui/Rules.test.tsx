// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Rules } from "./Rules";
import { MockPort } from "../port/mock";
import type { AgentPort, PermissionRules } from "../port/port";

afterEach(cleanup);

const rules = (granted: string[]): PermissionRules => ({
  mode: "ask", deny: [], ask: [], allow: [], path: "/home/u/.reasonix/config.toml", granted,
});

function draw(granted: string[], revoke = vi.fn(async () => rules([]))) {
  const port = {
    ...(new MockPort() as unknown as AgentPort),
    permissions: async () => rules(granted),
    revokeSessionGrant: revoke,
  } as unknown as AgentPort;
  render(<Rules port={port} onChanged={vi.fn()} />);
  return { revoke };
}

// "Allow for this session" is written down nowhere, so the file on screen is
// less than the agent may currently do — and before this there was no way back
// from one except ending the session.
describe("what a prompt allowed for this session", () => {
  it("lists the grants no file carries, and says they are this session only", async () => {
    draw(["Computer=com.example.Notes", "Browser=https://example.com"]);
    await screen.findByText("Computer=com.example.Notes");
    expect(screen.getByText("Browser=https://example.com")).toBeTruthy();
    expect(screen.getByText("本次会话另外允许了 2 项")).toBeTruthy();
    expect(screen.getByText("只在这个会话里有效，没有写进文件")).toBeTruthy();
  });

  it("takes one back by name, and all of them by none", async () => {
    const { revoke } = draw(["Computer=com.example.Notes"]);
    await screen.findByText("Computer=com.example.Notes");
    await userEvent.click(screen.getByLabelText("收回 Computer=com.example.Notes"));
    await waitFor(() => expect(revoke).toHaveBeenCalledWith("Computer=com.example.Notes"));

    cleanup();
    const all = draw(["Computer=com.example.Notes", "Browser=https://example.com"]);
    await screen.findByText("全部收回");
    await userEvent.click(screen.getByText("全部收回"));
    await waitFor(() => expect(all.revoke).toHaveBeenCalledWith(""));
  });

  it("says nothing where a session was given nothing", async () => {
    const { container } = render(
      <Rules port={{ ...(new MockPort() as unknown as AgentPort), permissions: async () => rules([]) } as unknown as AgentPort} onChanged={vi.fn()} />,
    );
    await waitFor(() => expect(container.querySelector(".rhead")).toBeTruthy());
    expect(container.querySelector(".granted")).toBeNull();
  });
});
