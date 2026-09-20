import { describe, expect, it } from "vitest";
import { parseDiff } from "./DiffView";

// 行号住在 @@ 头里。此前整段按 \n 切开、序号直接用下标，于是一处改在第 205 行的
// 改动在卡上标成 1、2、3 —— 而那个头本身还被当成代码渲染了出来。
describe("读一段 unified diff", () => {
  it("行号来自 @@ 头，不是数组下标", () => {
    const rows = parseDiff("@@ -61,2 +61,9 @@\n context\n+\tadded\n-\tremoved");
    // 删除行留空：它在改动后的文件里不存在，而这一列问的是「跳过去落在第几行」。
    expect(rows.map((r) => ("no" in r ? r.no : "hunk"))).toEqual([61, 62, null]);
    expect(rows.map((r) => ("sign" in r ? r.sign : "·"))).toEqual([" ", "+", "-"]);
  });

  it("@@ 头自己不进正文", () => {
    const rows = parseDiff("@@ -61,2 +61,9 @@\n+one");
    expect(rows).toHaveLength(1);
    expect("text" in rows[0] && rows[0].text).toBe("one");
  });

  // 缩进块的形状就是靠空行读出来的，filter(l => l.length > 0) 把它们整行丢掉了。
  it("保留空行", () => {
    const rows = parseDiff("@@ -1,3 +1,3 @@\n+a\n+\n+b");
    expect(rows).toHaveLength(3);
    expect("text" in rows[1] && rows[1].text).toBe("");
  });

  // 两处相隔几十行的改动此前连成一片，读起来像它们挨着。
  it("两段之间标出跳过了多少行", () => {
    const rows = parseDiff("@@ -1,1 +1,1 @@\n+a\n@@ -40,1 +40,1 @@\n+b");
    expect(rows.some((r) => "skipped" in r && r.skipped === 38)).toBe(true);
  });

  it("文件头不是内容", () => {
    const rows = parseDiff("diff --git a/x b/x\nindex 1..2\n--- a/x\n+++ b/x\n@@ -1,1 +1,1 @@\n+a");
    expect(rows).toHaveLength(1);
  });

  // 裸片段给不出真行号，就不给 —— 编一个比留空更坏。
  it("没有 @@ 头时不编行号", () => {
    const rows = parseDiff("+a\n-b");
    expect(rows.every((r) => "no" in r && r.no === null)).toBe(true);
  });
});
