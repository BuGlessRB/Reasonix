// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Account } from "./Account";
import { MockPort } from "../port/mock";
import type { AgentPort } from "../port/port";

afterEach(cleanup);

describe("account sign-in", () => {
  it("hands the approval URL to the host browser", async () => {
    const port = new MockPort() as unknown as AgentPort;
    port.accountLogin = async () => ({
      deviceCode: "device-secret",
      userCode: "ABCD-EFGH",
      verificationUri: "https://reasonix.io/device/",
      verificationUriComplete: "https://reasonix.io/device/?code=ABCD-EFGH",
      interval: 1,
      expiresIn: 0,
    });
    port.openExternal = vi.fn(async () => {});

    render(<Account port={port} state={{ signedIn: false }} reload={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() => expect(port.openExternal).toHaveBeenCalledWith("https://reasonix.io/device/?code=ABCD-EFGH"));
  });
});
