// 真机验证外观设置：字号、界面缩放、字体、壁纸，都要真的作用到页面上。
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { mkdirSync } from "node:fs";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const SHOTS = fileURLToPath(new URL("shots", import.meta.url));
mkdirSync(SHOTS, { recursive: true });

const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on("pageerror", (e) => fails.push("页面异常: " + e.message));

await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".app", { timeout: 20000 });
await page.waitForTimeout(600);

// 先灌几轮，让转录里有真正的正文可以量。
await page.evaluate(async () => {
  const y = () => new Promise((r) => requestAnimationFrame(r));
  for (let i = 0; i < 4; i++) {
    window.__feed({ kind: "turn_started" });
    window.__feed({ kind: "tool_dispatch", tool: { id: `t${i}`, name: "bash", args: '{"cmd":"ls"}' } });
    window.__feed({ kind: "tool_result", tool: { id: `t${i}`, name: "bash", args: '{"cmd":"ls"}', output: "a.go\nb.go\n", durationMs: 12 } });
    window.__feed({ kind: "text", text: `第 ${i} 段回答，用来量正文的字号有没有跟着走。\n\n` });
    window.__feed({ kind: "message" });
    window.__feed({ kind: "turn_done" });
    await y();
  }
});
await page.waitForTimeout(400);

// 设置页有吸顶头，普通点击会被它挡住；这些是内容区里的控件，直接派发点击。
const clickIn = (group, label) =>
  page.evaluate(
    ([g, l]) => {
      const box = [...document.querySelectorAll('[role="group"]')].find((e) => e.getAttribute("aria-label") === g);
      [...(box?.querySelectorAll("button") ?? [])].find((b) => b.textContent.trim() === l)?.click();
    },
    [group, label],
  );

const readPx = () =>
  page.evaluate(() => {
    const el = document.querySelector("#flowScroll .out .txt");
    return el ? parseFloat(getComputedStyle(el).fontSize) : 0;
  });
const rootVar = (name) => page.evaluate((n) => getComputedStyle(document.documentElement).getPropertyValue(n).trim(), name);

const base = await readPx();
check("默认正文字号", Math.abs(base - 13.5) < 0.6, `${base}px`);

// 打开设置 → 外观
await page.keyboard.press("Meta+Comma");
await page.waitForTimeout(500);
await page.evaluate(() => document.getElementById("prefs-appearance")?.click());
await page.waitForTimeout(400);
await page.screenshot({ path: `${SHOTS}/look-1-外观页.png` });

// 1) 正文字号
await clickIn("正文字号", "更大");
await page.waitForTimeout(500);
const bigger = await readPx();
check("正文调大生效", bigger > base + 2, `${base}px → ${bigger}px`);

// 2) 界面缩放。缩放后输入框还得在屏幕里 —— vh 不跟着除回去的话，
//    100vh 会溢出一个 zoom 倍，底部的输入框直接被顶出可视区。
const composerFits = () =>
  page.evaluate(() => {
    const el = document.querySelector(".compose");
    if (!el) return { ok: false, bottom: 0, vh: innerHeight };
    const r = el.getBoundingClientRect();
    return { ok: r.bottom <= innerHeight + 2 && r.top < innerHeight, bottom: Math.round(r.bottom), vh: innerHeight };
  });
for (const [label, want] of [["宽松", "1.15"], ["更大", "1.3"], ["紧凑", "0.9"]]) {
  await clickIn("界面大小", label);
  await page.waitForTimeout(500);
  const z = await page.evaluate(() => document.documentElement.style.zoom);
  check(`${label}缩放生效`, z === want, `zoom=${z || "(空)"}`);
  const fit = await composerFits();
  check(`${label}下输入框仍在屏幕里`, fit.ok, `底边 ${fit.bottom} / 视口 ${fit.vh}`);
}
await clickIn("界面大小", "宽松");
await page.waitForTimeout(400);
const zoom = await page.evaluate(() => document.documentElement.style.zoom);
await page.screenshot({ path: `${SHOTS}/look-2-放大后.png` });

// 3) 字体：写一个名字进去
await page.locator(".fontrow .fontown").first().fill("Georgia");
await page.waitForTimeout(600);
const ui = await rootVar("--ui");
check("自定义字体生效", ui.startsWith("Georgia,"), ui.slice(0, 46));
check("字体带兜底", ui.includes("sans-serif"), "写错名字不会把界面弄花");

// 3b) 等宽是另一个槽位，要单独验 —— 之前就是这里没测出来
await page.locator(".fontrow .fontown").nth(1).fill("Menlo");
await page.waitForTimeout(700);
const mono = await rootVar("--mono");
check("等宽字体生效", mono.startsWith("Menlo,"), mono.slice(0, 42));
const termFont = await page.evaluate(() => {
  const el = document.querySelector(".term");
  return el ? getComputedStyle(el).fontFamily : "";
});
check("终端块真的换了字", termFont.startsWith("Menlo"), termFont.slice(0, 42) || "(没有终端块)");

// 4) 壁纸：塞一张真图片进去
const shot = page.locator('input[type="file"]');
await shot.setInputFiles({
  name: "paper.png",
  mimeType: "image/png",
  // 一张 2x2 的 PNG，够验证整条上传—落盘—回读的链路
  buffer: Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFUlEQVR4nGP8z8Dwn4GBgYGJAQaAAAAA//8DAAKrAaXLmt0hAAAAAElFTkSuQmCC",
    "base64",
  ),
});
await page.waitForTimeout(900);
const bg = await rootVar("--bg-image");
check("壁纸挂上了", bg.startsWith("url("), bg.slice(0, 40));
check("背景层已开", (await page.evaluate(() => document.documentElement.dataset.bg)) === "on");
const alpha = await rootVar("--bg-alpha");
check("壁纸有可见的浓度", Number(alpha) > 0, `alpha=${alpha}`);
await page.screenshot({ path: `${SHOTS}/look-3-壁纸.png` });

// 5) 浓度滑块
const sliders = page.locator(".slider");
await sliders.first().fill("0.9");
await page.waitForTimeout(600);
check("浓度可调", Number(await rootVar("--bg-alpha")) > 0.8, `alpha=${await rootVar("--bg-alpha")}`);

// 6) 移除壁纸
await page.evaluate(() => [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "移除")?.click());
await page.waitForTimeout(700);
check("壁纸可移除", (await rootVar("--bg-image")) === "", `--bg-image="${await rootVar("--bg-image")}"`);
await page.screenshot({ path: `${SHOTS}/look-4-移除后.png` });

// 7) 重开页面：设置要还在（真的落了盘）。fixture 只存在内存里，所以这里
//    只验证没有落盘时不会残留脏状态；真机的持久化由 serve 的 config 负责。
await page.reload({ waitUntil: "networkidle" });
await page.waitForSelector(".app", { timeout: 20000 });
await page.waitForTimeout(700);
check("重开不残留壁纸", (await rootVar("--bg-image")) === "");

await browser.close();
console.log(fails.length ? `\n失败 ${fails.length} 项：\n- ${fails.join("\n- ")}` : "\n全部通过");
process.exit(fails.length ? 1 : 0);
