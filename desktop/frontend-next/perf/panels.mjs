// 两侧栏：收起要看得见地收，宽度要拖得动。
//
// 补间这件事没有报错可言 —— grid-template-columns 那一列里放回一个 minmax()，
// 动画就静静地退成跳变，界面照样能用。所以它得有人量。
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const RAIL = { def: 214, min: 168, max: 420 };
const SIDE = { def: 296 };

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

page.on("pageerror", (e) => fails.push("页面异常: " + e.message));

await page.goto(`${PAGE}?ws=2&sess=6`, { waitUntil: "networkidle" });
await page.waitForSelector(".app", { timeout: 15000 });
await page.waitForTimeout(700);

const widthOf = (sel) => page.evaluate((s) => Math.round(document.querySelector(s).getBoundingClientRect().width), sel);
const fold = (which) =>
  page.evaluate(
    (w) => dispatchEvent(new KeyboardEvent("keydown", { key: "\\", metaKey: true, shiftKey: w === "side", bubbles: true })),
    which,
  );

// 1) 收起与展开都补间：连着采一段帧，看落在两头之间的值有多少。
const sweep = (which) =>
  page.evaluate(async (w) => {
    const el = document.querySelector("." + w);
    const out = [];
    const tick = () => out.push(el.getBoundingClientRect().width);
    tick();
    dispatchEvent(new KeyboardEvent("keydown", { key: "\\", metaKey: true, shiftKey: w === "side", bubbles: true }));
    for (let i = 0; i < 45; i++) {
      await new Promise((r) => requestAnimationFrame(r));
      tick();
    }
    return out;
  }, which);

for (const [which, open] of [["rail", RAIL.def], ["side", SIDE.def]]) {
  const shut = await sweep(which);
  const between = new Set(shut.filter((w) => w > 0.5 && w < open - 0.5).map((w) => Math.round(w)));
  check(`收起 ${which} 是补间的`, between.size >= 6, `${between.size} 个中间宽度`);
  check(`收起 ${which} 收到底`, shut[shut.length - 1] === 0);

  const back = await sweep(which);
  const rising = new Set(back.filter((w) => w > 0.5 && w < open - 0.5).map((w) => Math.round(w)));
  check(`展开 ${which} 是补间的`, rising.size >= 6, `${rising.size} 个中间宽度`);
  check(`展开 ${which} 回到 ${open}`, Math.round(back[back.length - 1]) === open);
  await page.waitForTimeout(200);
}

// 2) 拖动改宽：跟着指针走，松手落在指针那里，且被记住。
const bar = await page.locator(".gutter-l").boundingBox();
check("左分隔条在场", !!bar);
await page.mouse.move(bar.x + bar.width / 2, bar.y + 300);
await page.mouse.down();
const trail = [];
for (const dx of [24, 56, 96]) {
  await page.mouse.move(bar.x + bar.width / 2 + dx, bar.y + 300);
  await page.waitForTimeout(60);
  trail.push(await widthOf(".rail"));
}
await page.mouse.up();
await page.waitForTimeout(200);
const dropped = await widthOf(".rail");
check("拖动跟手", trail[2] > trail[0], trail.join(" → "));
check("松手落在指针处", dropped === RAIL.def + 96, `${dropped}px`);
check("栏内内容跟着变宽", (await widthOf(".rail > *")) === dropped);
check(
  "宽度记在本地",
  (await page.evaluate(() => localStorage.getItem("rx-rail-w"))) === String(dropped),
);

// 3) 拖出来的宽度熬得过一次收起：展开回的是它，不是默认值。
await fold("rail");
await page.waitForTimeout(450);
check("拖过之后仍收得干净", (await widthOf(".rail")) === 0);
await fold("rail");
await page.waitForTimeout(450);
check("展开回到拖出来的宽度", (await widthOf(".rail")) === dropped, `${await widthOf(".rail")} vs ${dropped}`);

// 4) 上下限：越过就停住，不会把中间栏挤没。
const b2 = await page.locator(".gutter-l").boundingBox();
await page.mouse.move(b2.x + b2.width / 2, b2.y + 300);
await page.mouse.down();
await page.mouse.move(b2.x - 500, b2.y + 300);
await page.waitForTimeout(80);
const low = await widthOf(".rail");
await page.mouse.move(b2.x + 900, b2.y + 300);
await page.waitForTimeout(80);
const high = await widthOf(".rail");
await page.mouse.up();
check("拖不过下限", low === RAIL.min, `${low}px`);
check("拖不过上限", high === RAIL.max, `${high}px`);

// 5) 键盘也能调，且按住时不补间 —— 一步 12px 配一段 .34s 会追不上手。
await page.locator(".gutter-l").focus();
await page.keyboard.press("ArrowLeft");
await page.waitForTimeout(40);
check("方向键退一步", (await widthOf(".rail")) === high - 12, `${await widthOf(".rail")}px`);
check(
  "按键期间不补间",
  (await page.evaluate(() => getComputedStyle(document.querySelector(".app")).transitionProperty)) === "none",
);
await page.waitForTimeout(300);
check(
  "停手后补间回来",
  (await page.evaluate(() => getComputedStyle(document.querySelector(".app")).transitionProperty)) !== "none",
);
await page.keyboard.press("Home");
await page.waitForTimeout(450);
check("Home 回默认", (await widthOf(".rail")) === RAIL.def, `${await widthOf(".rail")}px`);

// 6) 窄到放不下时，栏和它的分隔条一起退场，页面不横向溢出。
const shown = (sel) => page.evaluate((s) => {
  const el = document.querySelector(s);
  return !!el && getComputedStyle(el).display !== "none";
}, sel);
for (const [w, keep] of [[1100, "只剩右边"], [780, "都不留"]]) {
  await page.setViewportSize({ width: w, height: 800 });
  await page.waitForTimeout(300);
  check(
    `窄到 ${w}px：${keep}`,
    (await shown(".gutter-l")) === false && (await shown(".gutter-r")) === (w > 840),
  );
  check(
    `窄到 ${w}px 不横向溢出`,
    await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
  );
}

await browser.close();
console.log(fails.length ? `\n失败 ${fails.length} 项：\n- ${fails.join("\n- ")}` : "\n全部通过");
process.exit(fails.length ? 1 : 0);
