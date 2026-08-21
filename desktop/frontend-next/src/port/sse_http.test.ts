import { describe, expect, it, vi } from "vitest";
import { HttpError } from "./port";
import { SseHttp } from "./sse_http";

class Probe extends SseHttp {
  call(path: string) {
    return this.post(path, {});
  }
}

function answer(status: number, body: string, contentType: string) {
  vi.stubGlobal("fetch", async () => ({
    ok: false,
    status,
    text: async () => body,
    json: async () => JSON.parse(body),
    headers: new Map([["content-type", contentType]]),
  }));
}

describe("a failed call", () => {
  // http.Error writes plain text, and parsing the body as JSON first threw the
  // only account of the failure away: users reported "/providers/edit: 500".
  it("carries a plain-text reason through", async () => {
    answer(500, "save user config: config is locked by another Reasonix process", "text/plain");
    const err = (await new Probe().call("/providers/edit").catch((e) => e)) as HttpError;
    expect(err).toBeInstanceOf(HttpError);
    expect(err.status).toBe(500);
    expect(err.message).toContain("locked by another Reasonix process");
  });

  it("still prefers the refusal envelope when there is one", async () => {
    answer(400, JSON.stringify({ code: "provider.no_models_picked", error: "pick at least one model" }), "application/json");
    const err = (await new Probe().call("/providers/edit").catch((e) => e)) as HttpError;
    expect(err.message).toBe("pick at least one model");
    expect(err.reason?.code).toBe("provider.no_models_picked");
  });

  // With nothing to report, the path and status are all a reader can be given.
  it("falls back to the path and status on an empty body", async () => {
    answer(502, "", "text/plain");
    const err = (await new Probe().call("/providers/edit").catch((e) => e)) as HttpError;
    expect(err.message).toBe("/providers/edit: 502");
  });
});
