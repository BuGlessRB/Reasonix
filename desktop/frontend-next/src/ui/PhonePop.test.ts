import { describe, expect, it } from "vitest";
import type { PairedDevice } from "../port/share";
import { presenceNote } from "./PhonePop";

const dev = (id: string, online: boolean): PairedDevice => ({ id, name: "", pairedAt: "", lastSeen: "", online });

describe("the phone card's presence note", () => {
  it("says a phone came online", () => {
    expect(presenceNote([dev("a", false)], [dev("a", true)])).toBe("设备 1 已连接");
  });

  it("says a phone went offline while still paired", () => {
    expect(presenceNote([dev("a", true), dev("b", true)], [dev("a", true), dev("b", false)])).toBe("设备 2 已断开");
  });

  it("says a phone that unpaired itself is gone, by the place it had", () => {
    expect(presenceNote([dev("a", true), dev("b", true)], [dev("a", true)])).toBe("设备 2 已断开");
  });

  it("says nothing when nothing changed", () => {
    expect(presenceNote([dev("a", true)], [dev("a", true)])).toBe("");
  });
});
