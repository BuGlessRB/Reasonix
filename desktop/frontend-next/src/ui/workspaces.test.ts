import { describe, expect, it } from "vitest";
import { removeHint } from "./Workspaces";

describe("removeHint", () => {
  it("says only what is true when nothing is open", () => {
    expect(removeHint(0, 0)).toBe("不会删除任何文件");
  });

  it("counts the panes the removal will close", () => {
    expect(removeHint(3, 0)).toBe("会先关掉 3 个面板；不会删除任何文件");
  });

  it("names the cost when closing would stop a turn", () => {
    expect(removeHint(3, 2)).toBe("会先关掉 3 个面板，其中 2 个还在跑；不会删除任何文件");
  });
});
