// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { PhonePop } from "./PhonePop";
import { HttpError } from "../port/http_error";
import type { HubPort } from "../port/hub";
import type { ShareStatus } from "../port/share";

afterEach(cleanup);

const status = (open: boolean): ShareStatus => ({ open, addresses: [{ ip: "10.0.0.2", interface: "en0", kind: "lan" }], devices: [] }) as ShareStatus;

function hubWith(reads: ShareStatus[], offerShare: () => Promise<unknown>) {
  let at = 0;
  const shareStatus = vi.fn(async () => reads[Math.min(at++, reads.length - 1)]);
  return { hub: { shareStatus, offerShare: vi.fn(offerShare) } as unknown as HubPort, shareStatus };
}

const closed = new HttpError(409, "share closed", { code: "share.closed" });

// The door was open when the window last looked and shut elsewhere since:
// opening the card reads first and does not ask a shut door for a code.
it("does not ask for a code when the read on opening finds the door shut", async () => {
  // Read on mount, read again as the open door starts being watched, then
  // shut by the time the card opens.
  const { hub, shareStatus } = hubWith([status(true), status(true), status(false)], () => Promise.reject(closed));
  render(<PhonePop hub={hub} />);
  await waitFor(() => expect(shareStatus).toHaveBeenCalledTimes(2));
  await userEvent.click(screen.getByRole("button", { name: "手机扫码访问" }));
  await waitFor(() => expect(shareStatus).toHaveBeenCalledTimes(3));
  expect(hub.offerShare).not.toHaveBeenCalled();
  expect(screen.queryByRole("alert")).toBeNull();
});

// A failure in the card is said in the card, not somewhere else on screen.
it("says a failed code request inside the card", async () => {
  const { hub } = hubWith([status(true)], () => Promise.reject(closed));
  render(<PhonePop hub={hub} />);
  await userEvent.click(await screen.findByRole("button", { name: "手机扫码访问" }));
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect(dialog.querySelector('[role="alert"]')?.textContent).toBe("手机访问已关闭，请先开启。"));
});
