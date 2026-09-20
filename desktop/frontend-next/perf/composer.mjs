// Composer geometry is interaction design: a control may exist in the DOM and
// still be unusable because a name pushed it away or a menu covers another one.
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&turns=4";
const BOX = 'textarea[role="combobox"]';
const fails = [];

const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: "dark", reducedMotion: "reduce" });
page.setDefaultTimeout(8000);
page.on("pageerror", (e) => fails.push("页面异常: " + e.message));

await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".compose");
await page.evaluate(() => document.fonts.ready);

const frame = () => page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
const geometry = () => page.evaluate(() => {
  const box = document.querySelector('textarea[role="combobox"]');
  const compose = document.querySelector(".compose");
  const send = document.querySelector('[data-action="session.send"]');
  const rect = (el) => el?.getBoundingClientRect();
  const hit = (el) => {
    const b = rect(el);
    if (!b) return false;
    const at = document.elementFromPoint(b.x + b.width / 2, b.y + b.height / 2);
    return !!at && (at === el || el.contains(at));
  };
  return { box: rect(box), compose: rect(compose), send: rect(send), sendHit: hit(send), fold: document.documentElement.dataset.fold ?? "" };
});

for (const { width, height, composeMax } of [
  { width: 1440, height: 900, composeMax: 150 },
  { width: 640, height: 900, composeMax: 150 },
  { width: 420, height: 520, composeMax: 220 },
]) {
  await page.setViewportSize({ width, height });
  await page.waitForTimeout(450);
  await page.fill(BOX, "");
  await frame();
  const g = await geometry();
  check(`${width}×${height}：空输入保持一行`, g.box.height <= 32, `输入 ${Math.round(g.box.height)}px`);
  check(`${width}×${height}：编辑器不过度占高`, g.compose.height <= composeMax, `编辑器 ${Math.round(g.compose.height)}px`);
  await page.fill(BOX, "继续检查");
  await frame();
  const ready = await geometry();
  check(`${width}×${height}：主动作可见且可点`, ready.sendHit && ready.send.right <= width && ready.send.bottom <= height, `fold=${ready.fold}`);
}

// A private gateway may accept an unpublished name much longer than anything
// in the public catalogue. It may be truncated, but may not cover its peers.
await page.setViewportSize({ width: 420, height: 520 });
await page.evaluate(() => {
  const name = document.querySelector('[data-action="model.select"] .nm');
  if (name) name.textContent = "vendor/internal/deepseek-flash-experimental-vision-20260910";
});
await frame();
const model = await page.evaluate(() => {
  const pick = document.querySelector('.turntools > .picker:first-of-type');
  const button = pick?.querySelector("button");
  const plan = document.querySelector('[data-action="plan.mode"]');
  const a = button?.getBoundingClientRect();
  const b = pick?.getBoundingClientRect();
  const c = plan?.getBoundingClientRect();
  return { button: a, picker: b, plan: c };
});
check("长模型名留在模型按钮内", model.button.width <= model.picker.width + 1, `按钮 ${Math.round(model.button.width)} / 槽 ${Math.round(model.picker.width)}`);
check("长模型名不覆盖计划入口", model.button.right <= model.plan.left + 1, `右缘 ${Math.round(model.button.right)} / 计划 ${Math.round(model.plan.left)}`);

// Moving from a completion to a toolbar menu is one layer change, not two
// translucent menus competing for the same text and pointer.
await page.setViewportSize({ width: 1010, height: 800 });
await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".compose");
await page.fill(BOX, "@");
await page.waitForSelector(".slashmenu");
await page.click('.turntools > .picker:first-of-type > button');
await page.waitForSelector(".slashmenu", { state: "detached" });
// 问控件自己开没开，不问菜单被画到了哪：这个菜单是 portal 出去的，挂在 body
// 下而不是 picker 里，所以按后代去找它只会等到超时 —— 而那时候互斥其实是成
// 立的。aria-expanded 是这个控件自己声明的状态，搬家搬不掉。
await page.waitForSelector('.turntools > .picker:first-of-type > button[aria-expanded="true"]');
check("补全与模型菜单互斥", await page.locator(".menu:not([hidden])").count() === 1);

await browser.close();
if (fails.length) {
  console.error(`\n${fails.length} 项不合格：\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\n编辑器关键几何与浮层互斥全部通过。");
