// 同一个内核拒绝，在两种语言下各说什么。内核只发码，话是这边挑的。
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (n, ok, d = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${n}${d ? "  — " + d : ""}`);
  if (!ok) fails.push(n);
};

const browser = await chromium.launch();

async function refuse(lang) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.addInitScript((l) => localStorage.setItem("rx-lang", l), lang);
  // 两侧摆一致，否则 adopt 会认为本地缓存过期并重载
  await page.goto(`${PAGE}?pref=${lang}`, { waitUntil: "networkidle" });
  await page.waitForSelector(".app", { timeout: 20000 });
  await page.waitForTimeout(700);
  await page.keyboard.press("Meta+Comma");
  await page.waitForTimeout(400);
  await page.evaluate(() => document.getElementById("prefs-appearance")?.click());
  await page.waitForTimeout(400);
  // 内核只认五种图片格式；TIFF 被拒，拒绝里带的是 wallpaper.unsupported_type
  await page.locator(".prefs input[type=\"file\"]").first().setInputFiles({
    name: "x.tiff",
    mimeType: "image/tiff",
    buffer: Buffer.from([0x49, 0x49, 0x2a, 0x00]),
  });
  await page.waitForTimeout(800);
  const said = await page.evaluate(
    () => document.querySelector('.find[data-lvl="err"] .t')?.textContent?.trim() ?? "",
  );
  await page.close();
  return said;
}

const zh = await refuse("zh");
const en = await refuse("en");
console.log(`\n  中文界面：${zh}\n  英文界面：${en}\n`);
check("中文界面说中文", zh.includes("这种图片格式用不了"), zh || "(空)");
check("英文界面说英文", /image format/i.test(en), en || "(空)");
check("同一个码，两种语言不同", zh !== en);
check("没把英文兜底漏给中文用户", !zh.includes("unsupported image type"));

await browser.close();
console.log(fails.length ? `\n失败 ${fails.length} 项` : "\n全部通过");
process.exit(fails.length ? 1 : 0);
