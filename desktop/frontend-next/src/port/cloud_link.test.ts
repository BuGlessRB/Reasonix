// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  CLOSE_IDLE,
  CLOSE_REAUTH_REQUIRED,
  CLOSE_REVOKED,
  closeAction,
  linkCodec,
  RemoteLink,
  RemoteLinkError,
  type RemoteConnection,
  type RemoteEnd,
} from "./cloud_link";

class FakeSocket extends EventTarget {
  readyState: number = WebSocket.OPEN;
  readonly sent: Array<Record<string, unknown>> = [];

  send(wire: string) {
    this.sent.push(JSON.parse(wire) as Record<string, unknown>);
  }

  close(code = 1000, reason = "") {
    if (this.readyState === WebSocket.CLOSED) return;
    this.readyState = WebSocket.CLOSED;
    this.dispatchEvent(Object.assign(new Event("close"), { code, reason }));
  }

  reply(id: string, body: string, extra: Record<string, unknown> = {}) {
    this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify({
      type: "desktop.response", id, index: 0, done: true, status: 200,
      body: linkCodec.bytesToBase64(new TextEncoder().encode(body)), ...extra,
    }) }));
  }
}

function connection(features: string[] = []): { conn: RemoteConnection; socket: FakeSocket } {
  const socket = new FakeSocket();
  return {
    socket,
    conn: {
      socket: socket as unknown as WebSocket,
      features,
      channel: {
        seal: async (value) => JSON.stringify(value),
        open: async (payload) => JSON.parse(payload) as Record<string, unknown>,
      },
    },
  };
}

function request(method = "GET", path = "/rt/one/events/replay") {
  return new Request(`${location.origin}${path}`, { method, body: method === "GET" ? undefined : "{}" });
}

let ends: RemoteEnd[];

beforeEach(() => {
  vi.useFakeTimers();
  ends = [];
});

afterEach(() => vi.useRealTimers());

describe("relay close codes", () => {
  it("reconnects on idle, legacy expiry and dropped sockets, and stops on revocation or sign-in", () => {
    expect(closeAction(CLOSE_IDLE)).toBe("reconnect");
    expect(closeAction(4003)).toBe("reconnect");
    expect(closeAction(1006)).toBe("reconnect");
    expect(closeAction(1012)).toBe("reconnect");
    expect(closeAction(CLOSE_REAUTH_REQUIRED)).toBe("reauth");
    expect(closeAction(CLOSE_REVOKED)).toBe("end");
    expect(closeAction(4004)).toBe("end");
  });
});

describe("remote link", () => {
  it("resends an in-flight read on the next connection and resolves it there", async () => {
    const first = connection();
    const second = connection();
    const dial = vi.fn(async () => second.conn);
    const link = new RemoteLink(first.conn, dial, (end) => ends.push(end));

    const answer = link.fetch(request());
    await vi.advanceTimersByTimeAsync(0);
    const id = String(first.socket.sent[0]?.id);
    first.socket.close(CLOSE_IDLE, "Idle timeout");
    await vi.advanceTimersByTimeAsync(1_000);

    expect(dial).toHaveBeenCalledOnce();
    expect(second.socket.sent[0]?.id).toBe(id);
    second.socket.reply(id, "late but whole");
    await expect((await answer).text()).resolves.toBe("late but whole");
    expect(ends).toEqual([]);
  });

  it("queues requests made while reconnecting and sends them once connected", async () => {
    const first = connection();
    const second = connection();
    const link = new RemoteLink(first.conn, async () => second.conn, (end) => ends.push(end));

    first.socket.close(4003, "Admission expired");
    const answer = link.fetch(request("POST", "/rt/one/send"));
    await vi.advanceTimersByTimeAsync(0);
    expect(first.socket.sent).toEqual([]);
    await vi.advanceTimersByTimeAsync(1_000);
    const id = String(second.socket.sent[0]?.id);
    second.socket.reply(id, "sent");
    await expect((await answer).text()).resolves.toBe("sent");
  });

  it("never resends a state change to a desktop that cannot recognise the resend", async () => {
    const first = connection();
    const second = connection();
    const link = new RemoteLink(first.conn, async () => second.conn, (end) => ends.push(end));

    const answer = link.fetch(request("POST", "/rt/one/send"));
    const rejected = answer.catch((error: Error) => error);
    await vi.advanceTimersByTimeAsync(0);
    first.socket.close(1006, "");
    await vi.advanceTimersByTimeAsync(1_000);

    expect(await rejected).toBeInstanceOf(Error);
    expect(second.socket.sent).toEqual([]);
  });

  it("resends a state change under the same id when the desktop replays answers", async () => {
    const first = connection(["replay"]);
    const second = connection(["replay"]);
    const link = new RemoteLink(first.conn, async () => second.conn, (end) => ends.push(end));

    const answer = link.fetch(request("POST", "/rt/one/send"));
    await vi.advanceTimersByTimeAsync(0);
    const id = String(first.socket.sent[0]?.id);
    first.socket.close(1006, "");
    await vi.advanceTimersByTimeAsync(1_000);

    expect(second.socket.sent[0]?.id).toBe(id);
    second.socket.reply(id, "once");
    await expect((await answer).text()).resolves.toBe("once");
  });

  it("asks for a sign-in instead of reconnecting when the relay says so", async () => {
    const first = connection();
    const dial = vi.fn();
    const link = new RemoteLink(first.conn, dial, (end) => ends.push(end));
    const pending = link.fetch(request()).catch((error: Error) => error);
    await vi.advanceTimersByTimeAsync(0);

    first.socket.close(CLOSE_REAUTH_REQUIRED, "Sign-in required");
    await vi.advanceTimersByTimeAsync(60_000);

    expect(dial).not.toHaveBeenCalled();
    expect(ends).toEqual([{ kind: "reauth", reason: "Sign-in required" }]);
    expect(await pending).toBeInstanceOf(Error);
  });

  it("stops for good when the device is revoked", async () => {
    const first = connection();
    const dial = vi.fn();
    new RemoteLink(first.conn, dial, (end) => ends.push(end));

    first.socket.close(CLOSE_REVOKED, "Access revoked");
    await vi.advanceTimersByTimeAsync(60_000);

    expect(dial).not.toHaveBeenCalled();
    expect(ends).toEqual([{ kind: "ended", reason: "Access revoked" }]);
  });

  it("ends with the account service's reason when a reconnect is refused", async () => {
    const first = connection();
    const dial = vi.fn()
      .mockRejectedValueOnce(new RemoteLinkError("transient", "relay busy"))
      .mockRejectedValueOnce(new RemoteLinkError("reauth", "sign in"));
    new RemoteLink(first.conn, dial, (end) => ends.push(end));

    first.socket.close(CLOSE_IDLE, "Idle timeout");
    await vi.advanceTimersByTimeAsync(5_000);

    expect(dial).toHaveBeenCalledTimes(2);
    expect(ends).toEqual([{ kind: "reauth", reason: "sign in" }]);
  });

  it("rebuilds the encrypted session when the desktop goes silent on an open socket", async () => {
    const first = connection();
    const second = connection();
    const dial = vi.fn(async () => second.conn);
    const link = new RemoteLink(first.conn, dial, (end) => ends.push(end), { requestTimeoutMs: 30_000 });

    const answer = link.fetch(request());
    await vi.advanceTimersByTimeAsync(30_000);
    await vi.advanceTimersByTimeAsync(1_000);

    expect(first.socket.readyState).toBe(WebSocket.CLOSED);
    expect(dial).toHaveBeenCalledOnce();
    const id = String(second.socket.sent[0]?.id);
    second.socket.reply(id, "fresh session");
    await expect((await answer).text()).resolves.toBe("fresh session");
  });

  it("gives up after its retries and says the connection ended", async () => {
    const first = connection();
    const dial = vi.fn(async () => { throw new RemoteLinkError("transient", "offline"); });
    new RemoteLink(first.conn, dial, (end) => ends.push(end), { retryDelaysMs: [10, 10] });

    first.socket.close(1006, "");
    await vi.advanceTimersByTimeAsync(100);

    expect(dial).toHaveBeenCalledTimes(2);
    expect(ends[0]?.kind).toBe("ended");
  });
});
