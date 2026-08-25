import { describe, expect, it, vi } from "vitest";
import { SseHub } from "./hub";
import { HttpError } from "./port";

function answer(status: number, body: string) {
  vi.stubGlobal("fetch", async () => ({
    ok: false,
    status,
    text: async () => body,
    json: async () => JSON.parse(body),
    headers: new Map([["content-type", "text/plain"]]),
  }));
}

// The same defect SseHttp was fixed for, still standing in the hub port: asking
// for JSON first threw away every answer that was not an envelope, and what
// reached the window was "this request did not reach the kernel (HTTP 502)" —
// wrong twice over, because it had, and the kernel had explained itself.
describe("a failed hub call", () => {
  it("carries a plain-text reason through", async () => {
    answer(502, "remote /runtimes: 400 Bad Request: workspace /srv/data is not a directory");
    const err = (await new SseHub().openRemote({ host: "gpu-box" }).catch((e) => e)) as HttpError;
    expect(err).toBeInstanceOf(HttpError);
    expect(err.status).toBe(502);
    expect(err.detailed).toBe(true);
    expect(err.message).toContain("not a directory");
  });

  it("still prefers the refusal envelope when there is one", async () => {
    answer(502, JSON.stringify({
      code: "remote.kernel_refused",
      error: "remote /runtimes: 400 Bad Request",
      params: { host: "gpu-box", detail: "workspace /srv/data is not a directory" },
    }));
    const err = (await new SseHub().openRemote({ host: "gpu-box" }).catch((e) => e)) as HttpError;
    expect(err.reason?.code).toBe("remote.kernel_refused");
    expect(err.reason?.params?.host).toBe("gpu-box");
    expect(err.detailed).toBe(true);
  });

  // Nothing to report is the one case where a path and a status are all there
  // is, and the only one that may say the request never arrived.
  it("falls back to the path and status on an empty body", async () => {
    answer(502, "");
    const err = (await new SseHub().openRemote({ host: "gpu-box" }).catch((e) => e)) as HttpError;
    expect(err.message).toBe("/remotes/open: 502");
    expect(err.detailed).toBe(false);
  });
});
