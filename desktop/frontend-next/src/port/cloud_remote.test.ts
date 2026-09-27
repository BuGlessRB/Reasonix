// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { REMOTE_DISCONNECTED_EVENT, remoteCodec, remoteConnectionEnded } from "./cloud_remote";

describe("remote Studio binary framing", () => {
  it("round-trips binary bodies and joins response chunks in index order", () => {
    const source = new Uint8Array([0, 1, 2, 127, 128, 254, 255]);
    const encoded = remoteCodec.bytesToBase64(source);
    expect(remoteCodec.base64ToBytes(encoded)).toEqual(source);
    const joined = remoteCodec.concatChunks(new Map([
      [1, remoteCodec.bytesToBase64(source.subarray(4))],
      [0, remoteCodec.bytesToBase64(source.subarray(0, 4))],
    ]));
    expect(joined).toEqual(source);
  });

  it("announces a relay disconnect to the Web Studio shell", () => {
    let reason = "";
    window.addEventListener(REMOTE_DISCONNECTED_EVENT, (event) => {
      reason = (event as CustomEvent<{ reason: string }>).detail.reason;
    }, { once: true });
    remoteCodec.announceClosed("Disconnected by device");
    expect(reason).toBe("Disconnected by device");
    expect(remoteConnectionEnded()).toBe(true);
  });
});
