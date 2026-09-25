// Motion has two jobs here: acknowledge an action, or report live work. This
// guard keeps decoration out of the second job and old facts out of the first.
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?ws=1&sess=1&turns=4&pref=zh";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();

const runningLoops = (page) => page.evaluate(() =>
  document.getAnimations({ subtree: true }).filter((a) => {
    if (a.playState !== "running" || a.effect?.getTiming().iterations !== Infinity) return false;
    const el = a.effect?.target;
    if (!(el instanceof Element)) return false;
    const box = el.getBoundingClientRect();
    const s = getComputedStyle(el);
    return s.visibility !== "hidden" && s.display !== "none" && box.width > 0 && box.height > 0;
  }).map((a) => a.animationName),
);

const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: "dark" });
await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".compose");

const located = await page.evaluate(() => {
  const card = [...document.querySelectorAll(".call[data-k='write']")].at(-1);
  const head = card?.querySelector(":scope > .c > .tool-disclosure > .hl, :scope > .c > .hl");
  const name = head?.querySelector(".nm");
  const arg = head?.querySelector(".arg");
  if (!(card instanceof HTMLElement) || !(name instanceof HTMLElement) || !(arg instanceof HTMLElement)) return null;
  const before = getComputedStyle(card).backgroundColor;
  card.setAttribute("data-hit", "");
  return {
    before,
    cardAnimations: card.getAnimations({ subtree: false }).map((a) => a.animationName),
    cardBackground: getComputedStyle(card).backgroundColor,
    nameAnimation: getComputedStyle(name).animationName,
    argAnimation: getComputedStyle(arg).animationName,
  };
});
check(
  "定位 Update 时只强调标题而不闪整张输出",
  located?.cardAnimations.length === 0
    && located.cardBackground === located.before
    && located.nameAnimation === "hitmark"
    && located.argAnimation === "hitmark",
  JSON.stringify(located),
);
await page.evaluate(() => {
  window.__feed({ kind: "turn_started" });
  window.__feed({ kind: "tool_dispatch", tool: { id: "motion-live", name: "read_file", args: '{"path":"motion.ts"}', readOnly: true } });
});
await page.waitForTimeout(120);
const loops = await runningLoops(page);
check("运行态只保留两个核心循环信号", loops.length <= 2, loops.join(", "));

await page.evaluate(() => window.__feed({
  kind: "approval_request",
  approval: { id: "motion-approval", tool: "bash", subject: "npm test" },
}));
await page.waitForTimeout(120);
const decision = await page.evaluate(() => {
  const composer = document.querySelector(".compose");
  const probe = document.createElement("i");
  probe.style.color = "var(--border)";
  document.body.append(probe);
  const neutral = getComputedStyle(probe).color;
  probe.style.color = "color-mix(in srgb, var(--accent) 55%, var(--border))";
  const accent = getComputedStyle(probe).color;
  probe.remove();
  return {
    ask: document.querySelector(".apv:not([data-sealed]), .ask") !== null,
    edge: getComputedStyle(composer, "::after").borderTopColor,
    neutral,
    accent,
  };
});
check("等待决定时只有决定卡承担大面积强调", decision.ask && decision.edge !== decision.accent, `${decision.edge} / ${decision.neutral}`);

await page.setViewportSize({ width: 420, height: 800 });
await page.waitForTimeout(320);
const narrow = await page.evaluate(() => ({
  lane: getComputedStyle(document.querySelector(".pane")).getPropertyValue("--srail-w").trim(),
  oneLine: document.querySelector(".crumb").getBoundingClientRect().height <= 24,
  overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
}));
check("窄屏消息定位轨让出正文宽度", narrow.lane === "24px", narrow.lane);
check("窄屏面包屑保持单行", narrow.oneLine);
check("窄屏没有横向溢出", narrow.overflow <= 1, `${narrow.overflow}px`);

await page.locator('[data-pane="flow"]').focus();
await page.keyboard.press("End");
await page.waitForTimeout(80);
check("消息定位轨可用键盘滚动", await page.evaluate(() => document.querySelector('[data-pane="flow"]').scrollTop > 0));

const reduced = await browser.newPage({
  viewport: { width: 1440, height: 900 },
  colorScheme: "dark",
  reducedMotion: "reduce",
});
await reduced.goto(PAGE, { waitUntil: "networkidle" });
await reduced.waitForSelector(".compose");
await reduced.evaluate(() => {
  window.__feed({ kind: "turn_started" });
  window.__feed({ kind: "tool_dispatch", tool: { id: "motion-still", name: "read_file", args: '{"path":"still.ts"}', readOnly: true } });
});
await reduced.waitForTimeout(80);
const stillLoops = await runningLoops(reduced);
check("减少动态效果时没有循环动画", stillLoops.length === 0, stillLoops.join(", "));

await browser.close();
console.log(fails.length ? `\n失败 ${fails.length} 项：\n- ${fails.join("\n- ")}` : "\n动效节奏、窄屏与减弱动态全部通过。");
process.exit(fails.length ? 1 : 0);
