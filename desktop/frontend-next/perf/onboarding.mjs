// The first-run "start" button must be reachable at every window height.
// The body clips, so a card taller than the window is only reachable if the
// stage itself scrolls. Focus is no evidence: the browser scrolls the clipped
// body to a focused element, so the wheel is what gets asserted.
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?onboarding=1";
const SIZES = [[1920, 700], [1600, 600], [2560, 560], [1366, 600], [1280, 540], [1440, 900], [1600, 760, 1.25]];
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const [width, height, zoom = 1] of SIZES) {
    const ctx = await browser.newContext({ locale: "zh-CN", viewport: { width, height }, colorScheme: scheme });
    const page = await ctx.newPage();
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    await page.goto(PAGE, { waitUntil: "domcontentloaded" });
    await page.waitForSelector(".onb-card");
    if (zoom !== 1) {
      await page.evaluate((z) => {
        document.documentElement.style.setProperty("zoom", String(z));
        document.documentElement.style.setProperty("--zoom", String(z));
      }, zoom);
    }
    const tag = `${scheme} ${width}x${height}${zoom !== 1 ? ` zoom${zoom}` : ""}`;

    const reachable = async (label, enabled) => {
      const go = page.locator(".onb-go");
      await page.evaluate(() => document.activeElement?.blur());
      await page.mouse.move(width / 2, height / 2);
      await page.mouse.wheel(0, 4000);
      await page.waitForTimeout(250);
      const box = await go.boundingBox();
      check(`${tag} ${label}: after wheeling to the bottom the button is in the viewport`, !!box && box.y >= 0 && box.y + box.height <= height + 0.5, JSON.stringify(box));
      const bar = await page.locator(".onb-brandbar").boundingBox();
      check(`${tag} ${label}: the title bar stays in the viewport`, !!bar && bar.y >= -0.5 && bar.y + bar.height <= height + 0.5, JSON.stringify(bar));
      if (!enabled) return;
      const hit = await page.evaluate(() => {
        const r = document.querySelector(".onb-go").getBoundingClientRect();
        const el = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
        return !!el?.closest(".onb-go");
      });
      check(`${tag} ${label}: the point under its centre hits it`, hit);
      const first = page.locator(".onb-chip").first();
      await page.evaluate(() => document.activeElement?.blur());
      await first.focus();
      await page.waitForTimeout(250);
      const [fb, bb] = [await first.boundingBox(), await page.locator(".onb-brandbar").boundingBox()];
      check(`${tag} ${label}: a focused top-of-card element sits below the title bar`, !!fb && !!bb && fb.y >= bb.y + bb.height - 0.5, JSON.stringify({ fb, bb }));
    };

    await reachable("before connect", false);
    await page.fill('input[placeholder="sk-…"]', "sk-test");
    await page.locator(".onb-go").click();
    await page.waitForSelector(".onb-found");
    await reachable("after connect", true);
    await ctx.close();
  }
}
await browser.close();
console.log(fails.length ? `\n${fails.length} failed` : "\nall passed");
process.exit(fails.length ? 1 : 0);
