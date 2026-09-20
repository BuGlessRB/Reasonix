// Card quality is mostly a set of invariants: prose must stay in its column,
// machine output owns its scrollport, and decision actions remain reachable.
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&ws=2&sess=2&turns=1";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: "zh-CN", colorScheme: "dark" });
page.setDefaultTimeout(8000);
page.on("pageerror", (e) => fails.push("页面异常: " + e.message));

await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".compose");
await page.evaluate(() => {
  const long = "https://private.example/" + "unpublished-deepseek-flash-vision-".repeat(45);
  window.__feed({ kind: "turn_started" });
  window.__feed({ kind: "text", text: long });
  window.__feed({ kind: "message" });
  window.__feed({ kind: "tool_dispatch", tool: { id: "read-ok", name: "read_file", args: '{"path":"a.ts"}', readOnly: true } });
  window.__feed({ kind: "tool_result", tool: { id: "read-ok", name: "read_file", args: '{"path":"a.ts"}', output: "1→ok", readOnly: true } });
  window.__feed({ kind: "tool_dispatch", tool: { id: "read-bad", name: "read_file", args: '{"path":"b.ts"}', readOnly: true } });
  window.__feed({ kind: "tool_result", tool: { id: "read-bad", name: "read_file", args: '{"path":"b.ts"}', err: "no such file", readOnly: true } });
  window.__feed({ kind: "tool_dispatch", tool: { id: "long-output", name: "bash", args: "run", readOnly: false } });
  window.__feed({ kind: "tool_result", tool: { id: "long-output", name: "bash", args: "run", output: "x".repeat(1800) + "\n" + "row\n".repeat(80), readOnly: false } });
  window.__feed({ kind: "approval_request", approval: { id: "approval-layout", tool: "bash", subject: long } });
  window.__feed({ kind: "ask_request", ask: { id: "ask-layout", questions: [
    { id: "q1", header: "一个很长的问题标题", prompt: "选择一种方案", options: [{ label: "方案 A" }, { label: "方案 B" }] },
    { id: "q2", header: "另一个同样很长的问题标题", prompt: "补充选择", options: [{ label: "继续" }, { label: "停止" }] },
    { id: "q3", header: "第三个问题标题", prompt: "最后一个选择", options: [{ label: "保留" }, { label: "删除" }] },
  ] } });
});

for (const { width, height } of [{ width: 1440, height: 900 }, { width: 640, height: 800 }, { width: 420, height: 640 }]) {
  await page.setViewportSize({ width, height });
  await page.waitForTimeout(500);
  await page.locator(".ask").scrollIntoViewIfNeeded();
  const layout = await page.evaluate(() => {
    const box = (selector) => document.querySelector(selector)?.getBoundingClientRect();
    const prose = document.querySelector('.call[data-k="say"] .out .txt');
    const flow = document.querySelector('[data-pane="flow"]');
    const ask = document.querySelector(".ask");
    const tabs = document.querySelector(".ask-tabs");
    const foot = document.querySelector(".ask-foot");
    const primary = foot?.querySelector(".btn");
    const secondary = foot?.querySelector(".dismiss");
    const apv = document.querySelector(".apv:not([data-sealed])");
    const actions = [...(apv?.querySelectorAll(".apv-ft .btn") ?? [])].map((el) => el.getBoundingClientRect());
    return {
      flowOverflow: flow ? flow.scrollWidth - flow.clientWidth : 999,
      proseOverflow: prose ? prose.scrollWidth - prose.clientWidth : 999,
      askOverflow: ask ? ask.scrollWidth - ask.clientWidth : 999,
      tabsOverflow: getComputedStyle(tabs).overflowX,
      primary: box(".ask-foot .btn"),
      secondary: box(".ask-foot .dismiss"),
      foot: box(".ask-foot"),
      approvalRows: [...new Set(actions.map((r) => Math.round(r.y)))].length,
      approvalMainWidth: actions[0]?.width ?? 0,
      approvalWidth: apv?.getBoundingClientRect().width ?? 0,
    };
  });
  check(`${width}px：正文不横向溢出`, layout.proseOverflow <= 1, `${Math.round(layout.proseOverflow)}px`);
  check(`${width}px：提问卡不撑宽`, layout.askOverflow <= 1, `${Math.round(layout.askOverflow)}px`);
  check(`${width}px：转录不横向溢出`, layout.flowOverflow <= 1, `${Math.round(layout.flowOverflow)}px`);
  check(`${width}px：问题标签可横向到达`, layout.tabsOverflow === "auto");
  if (width === 420) {
    check("420px：提问主次动作分行", layout.primary.bottom <= layout.secondary.top + 1);
    check("420px：提问动作都留在卡内", layout.secondary.right <= layout.foot.right + 1);
    check("420px：审批主操作独占首行", layout.approvalRows === 2 && layout.approvalMainWidth >= layout.approvalWidth * 0.8,
      `${layout.approvalRows} 行 / ${Math.round(layout.approvalMainWidth)}px`);
  }
}

const output = await page.evaluate(() => {
  const body = document.querySelector('[data-call="long-output"] .out-body');
  if (!body) return null;
  body.scrollLeft = body.scrollWidth;
  return { overflow: getComputedStyle(body).overflowX, left: body.scrollLeft, extra: body.scrollWidth - body.clientWidth };
});
check("长工具输出在折叠态即可横向阅读", output?.overflow === "auto" && output.left > 0 && output.extra > 0,
  output ? `${Math.round(output.extra)}px 可滚` : "没有输出视口");
check("合并读取仍显示失败", await page.locator(".call .fail", { hasText: "项失败" }).count() === 1);
check("回执没有重复根容器", await page.locator(".rc > .rc").count() === 0);

await page.getByRole("tab", { name: "任务", exact: true }).click();
await page.waitForTimeout(400);
const task = await page.evaluate(() => {
  const rows = [...document.querySelectorAll(".tk-acts button")];
  return {
    nestedBoxes: rows.filter((row) => {
      const s = getComputedStyle(row);
      return parseFloat(s.borderLeftWidth) > 0 || parseFloat(s.borderRightWidth) > 0;
    }).length,
    overflow: document.querySelector(".tkv")?.scrollWidth - document.querySelector(".tkv")?.clientWidth,
    empty: document.querySelector(".tk-now h2")?.textContent ?? "",
  };
});
check("快速操作是轻量行，不再卡中套卡", task.nestedBoxes === 0);
check("任务看板不横向溢出", (task.overflow ?? 999) <= 1);
check("空目标是状态，不是输入指令", task.empty.includes("尚未生成"));

await browser.close();
if (fails.length) {
  console.error(`\n${fails.length} 项不合格：\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\n卡片层级、状态和窄栏几何全部通过。");
