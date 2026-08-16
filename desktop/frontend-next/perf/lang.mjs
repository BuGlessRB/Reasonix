import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { mkdirSync, readFileSync } from "node:fs";
import { join, dirname } from "node:path";
const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const SHOTS = fileURLToPath(new URL("shots", import.meta.url));
// PERF_SRC lets the script run from outside the repo (a scratch copy) and
// still read the fixtures it must exempt.
const SRC = process.env.PERF_SRC ?? join(dirname(fileURLToPath(import.meta.url)), "..", "src");
mkdirSync(SHOTS, { recursive: true });
const fails = [];
const check = (n, ok, d = "") => { console.log(`${ok ? "  ok" : "FAIL"}  ${n}${d ? "  — " + d : ""}`); if (!ok) fails.push(n); };

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on("pageerror", (e) => fails.push("页面异常: " + e.message));

// 先按英文启动
await page.addInitScript(() => localStorage.setItem("rx-lang", "en"));
await page.goto(`${PAGE}?pref=en`, { waitUntil: "networkidle" });
await page.waitForSelector(".app", { timeout: 20000 });
await page.waitForTimeout(800);

check("<html lang> 是 en", (await page.evaluate(() => document.documentElement.lang)) === "en");

await page.evaluate(async () => {
  const y = () => new Promise((r) => requestAnimationFrame(r));
  window.__feed({ kind: "turn_started" });
  window.__feed({ kind: "tool_dispatch", tool: { id: "t1", name: "edit_file", args: '{"path":"pkg/a.go"}' } });
  window.__feed({ kind: "tool_result", tool: { id: "t1", name: "edit_file", args: '{"path":"pkg/a.go"}', output: "done", durationMs: 210, added: 4, removed: 2 } });
  window.__feed({ kind: "text", text: "A short answer.\n\n" });
  window.__feed({ kind: "message" });
  await y();
});
await page.waitForTimeout(500);

const seen = (s) => page.locator(`text=${s}`).first().isVisible().catch(() => false);
// 固件替用户造的数据不该翻：技能描述、钩子名、记忆条目、会话标题，真机上
// 都是用户自己写的内容。豁免表直接从固件源码里读，而不是手写一串正则 ——
// 固件改了，豁免自动跟着改。语言名同理：英文界面里「中文」就该是「中文」。
const fixtureSrc = ["port/fixture.ts", "port/mock.ts", "port/mock_ext.ts", "port/mock_shell.ts", "port/mock_hub.ts"]
  .map((p) => { try { return readFileSync(join(SRC, p), "utf8"); } catch { return ""; } })
  .join("\n");
const FIXTURE_TEXT = new Set(
  [...fixtureSrc.matchAll(/(["'`])((?:[^"'`\\]|\\.)*?)\1/g)].map((m) => m[2]).filter((x) => /[一-鿿]/.test(x)),
);
const isFixture = (s) => s === "中文" || [...FIXTURE_TEXT].some((f) => f.includes(s) || s.includes(f));
check("侧栏译成英文", await seen("Metrics"));
check("标签页译成英文", await seen("Activity"));
check("会话树译成英文", await seen("Workspaces"));
check("面板译成英文", await seen("Pending changes"));
check("子代理面板译成英文", await seen("Subagents"));
await page.screenshot({ path: `${SHOTS}/lang-en.png` });

// 设置页：逐个分区看有没有漏译
await page.keyboard.press("Meta+Comma");
await page.waitForTimeout(500);
const sections = ["session", "model", "tools", "hooks", "ext", "network", "memory", "account", "versions", "appearance"];
const leftover = {};
for (const sec of sections) {
  await page.evaluate((id) => document.getElementById(`prefs-${id}`)?.click(), sec);
  await page.waitForTimeout(320);
  const zh = await page.evaluate(() => {
    const cjk = /[一-鿿]/;
    const out = [];
    const walk = (n) => {
      if (n.nodeType === 3) { const s = n.textContent.trim(); if (cjk.test(s)) out.push(s); return; }
      if (n.nodeType !== 1 || n.tagName === "SCRIPT") return;
      for (const c of n.childNodes) walk(c);
    };
    for (const sel of [".prefs-main", ".prefs-nav"]) {
      const el = document.querySelector(sel);
      if (el) walk(el);
    }
    return [...new Set(out)];
  });
  // 固件造的「用户数据」不该翻：技能与钩子的名字、记忆条目、代理地址、
  // 会话标题，真机上都是用户自己写的。语言名同理 —— 英文界面里「中文」
  // 就该是「中文」。
  const own = zh.filter((x) => !isFixture(x));
  if (own.length) leftover[sec] = own.map((x) => x.slice(0, 34));
}
await page.screenshot({ path: `${SHOTS}/lang-en-settings.png` });
const secBad = Object.keys(leftover).length;
check("设置各分区已译", secBad === 0, secBad ? Object.entries(leftover).map(([k, v]) => `${k}: ${v.slice(0,3).join("/")}`).join("  |  ") : "10 个分区都干净");
await page.keyboard.press("Escape");
await page.waitForTimeout(300);

// 界面里不该再有中文（fixture 造的会话标题除外）
const stray = await page.evaluate(() => {
  const cjk = /[一-鿿]/;
  const skip = new Set(["SCRIPT", "STYLE"]);
  const out = [];
  const walk = (n) => {
    if (n.nodeType === 3) { const s = n.textContent.trim(); if (cjk.test(s)) out.push(s); return; }
    if (n.nodeType !== 1 || skip.has(n.tagName)) return;
    for (const c of n.childNodes) walk(c);
  };
  walk(document.querySelector(".app"));
  return [...new Set(out)];
});
const strayOwn = stray.filter((x) => !isFixture(x));
check("主界面无残留中文", strayOwn.length === 0, strayOwn.length ? strayOwn.slice(0, 6).map((x) => x.slice(0, 30)).join(" / ") : `${stray.length} 处全是固件数据`);

// 切回中文：另开一页，否则上面那条 initScript 每次导航都会把语言写回 en
const zh = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await zh.addInitScript(() => localStorage.setItem("rx-lang", "zh"));
await zh.goto(`${PAGE}?pref=zh`, { waitUntil: "networkidle" });
await zh.waitForSelector(".app", { timeout: 20000 });
await zh.waitForTimeout(700);
check("切回中文", await zh.locator("text=度量").first().isVisible().catch(() => false));
check("<html lang> 回到 zh-CN", (await zh.evaluate(() => document.documentElement.lang)) === "zh-CN");
await zh.screenshot({ path: `${SHOTS}/lang-zh.png` });

await browser.close();
console.log(fails.length ? `\n失败 ${fails.length} 项：\n- ${fails.join("\n- ")}` : "\n全部通过");
