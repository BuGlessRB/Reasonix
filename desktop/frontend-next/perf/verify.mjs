// 验证优化没有改掉行为：跟随、脱离跟随、回到最新、卡片渲染、切页往返。
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { mkdirSync } from "node:fs";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const SHOTS = fileURLToPath(new URL("shots", import.meta.url));

mkdirSync(SHOTS, { recursive: true });
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

page.on("pageerror", (e) => fails.push("页面异常: " + e.message));
page.on("console", (m) => m.type() === "error" && fails.push("控制台错误: " + m.text()));

await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".app", { timeout: 15000 });
await page.waitForTimeout(700);

// 灌 60 轮，制造一条需要滚动的记录。
await page.evaluate(async (n) => {
  const yieldFrame = () => new Promise((r) => requestAnimationFrame(r));
  for (let i = 0; i < n; i++) {
    const id = `t${i}`;
    window.__feed({ kind: "turn_started" });
    window.__feed({ kind: "tool_dispatch", tool: { id, name: "edit_file", args: JSON.stringify({ path: `pkg/f${i}.go` }) } });
    window.__feed({ kind: "tool_result", tool: { id, name: "edit_file", args: JSON.stringify({ path: `pkg/f${i}.go` }), output: "写入完成", durationMs: 210, added: 4, removed: 2 } });
    window.__feed({ kind: "text", text: `第 ${i} 段回答。\n\n- 要点一\n- 要点二\n\n` });
    window.__feed({ kind: "message" });
    window.__feed({ kind: "turn_done" });
    if (i % 10 === 9) await yieldFrame();
  }
  await yieldFrame();
}, 60);
await page.waitForTimeout(500);

const flow = page.locator("#flowScroll").first();
const geom = () => flow.evaluate((el) => ({ top: el.scrollTop, h: el.scrollHeight, c: el.clientHeight }));

// 1) 挂载的块真的画出来了，而不是留下一片占位空白。
const cards = await page.locator("#flowScroll .call").count();
const blocks = await page.locator("#flowScroll .chunk").count();
check("底部卡片已挂载", cards > 0, `${cards} 张 / ${blocks} 块`);
check("远处的块已卸载", cards < blocks * 48, `挂载 ${cards}，全挂会是 ${blocks * 48}`);
const visibleText = await flow.evaluate((el) => (el.innerText || "").trim().length);
check("可见文字非空", visibleText > 200, `${visibleText} 字`);

// 2) 灌完之后停在底部。
const g1 = await geom();
check("自动停在底部", g1.h - g1.top - g1.c < 60, `距底 ${(g1.h - g1.top - g1.c).toFixed(0)}px`);

// 3) 流式增量继续时保持跟随。
await page.evaluate(async () => {
  window.__feed({ kind: "turn_started" });
  for (let i = 0; i < 40; i++) {
    window.__feed({ kind: "text", text: "继续写下去的一段话。" });
    await new Promise((r) => requestAnimationFrame(r));
  }
});
await page.waitForTimeout(300);
const g2 = await geom();
check("流式时保持跟随", g2.h - g2.top - g2.c < 60, `距底 ${(g2.h - g2.top - g2.c).toFixed(0)}px`);
await page.screenshot({ path: `${SHOTS}/1-跟随中.png` });

// 4) 往上滚 → 脱离跟随，「回到最新」出现，新增量不再把视口拽回去。
await flow.evaluate((el) => el.scrollTo({ top: el.scrollHeight * 0.35 }));
await page.waitForTimeout(400);
const away = await geom();
const jump = page.locator("button.jump").first();
check("向上滚后按钮出现", await jump.isVisible(), `scrollTop=${away.top.toFixed(0)}`);
await page.evaluate(async () => {
  for (let i = 0; i < 30; i++) {
    window.__feed({ kind: "text", text: "又写了一些新的内容。" });
    await new Promise((r) => requestAnimationFrame(r));
  }
  window.__feed({ kind: "turn_done" });
});
await page.waitForTimeout(400);
const stay = await geom();
check("脱离跟随后不被拽回", Math.abs(stay.top - away.top) < 80, `位移 ${Math.abs(stay.top - away.top).toFixed(0)}px`);
await page.screenshot({ path: `${SHOTS}/2-已脱离跟随.png` });

// 4b) 往上滚到远处，历史块要挂载出来，且文档高度不能塌。
const tallBefore = (await geom()).h;
await flow.evaluate((el) => el.scrollTo({ top: el.scrollHeight * 0.08 }));
await page.waitForTimeout(700);
const upTop = await page.evaluate(() => {
  const el = document.querySelector("#flowScroll");
  const cards = [...el.querySelectorAll(".call")];
  const r = el.getBoundingClientRect();
  return cards.filter((c) => { const b = c.getBoundingClientRect(); return b.bottom > r.top && b.top < r.bottom; }).length;
});
check("滚到远处会挂载历史块", upTop > 0, `视口内 ${upTop} 张`);
const tallAfter = (await geom()).h;
check("滚动时文档高度稳定", Math.abs(tallAfter - tallBefore) / tallBefore < 0.15, `${tallBefore.toFixed(0)} → ${tallAfter.toFixed(0)}px`);
await page.screenshot({ path: `${SHOTS}/2b-滚到历史处.png` });

// 5) 点「回到最新」回到底部并恢复跟随。
await jump.click();
await page.waitForTimeout(500);
const back = await geom();
check("回到最新可用", back.h - back.top - back.c < 60, `距底 ${(back.h - back.top - back.c).toFixed(0)}px`);

// 6) 切到轨迹再切回，记录仍在且仍贴底。
await page.locator('[role="tab"]').nth(1).click();
await page.waitForTimeout(300);
const trajRows = await page.locator("table.traj tbody tr").count();
check("轨迹页有内容", trajRows > 50, `${trajRows} 行`);
await page.screenshot({ path: `${SHOTS}/3-轨迹页.png` });
await page.locator('[role="tab"]').nth(0).click();
await page.waitForTimeout(400);
const after = await page.locator("#flowScroll .call").count();
check("切回后卡片仍在", after > 0, `${after} 张`);
await page.screenshot({ path: `${SHOTS}/4-切回活动页.png` });

// 7) 审批卡这类交互卡仍然可答。
await page.evaluate(() => {
  window.__feed({ kind: "turn_started" });
  window.__feed({ kind: "approval_request", approval: { id: "a1", tool: "bash", subject: "rm -rf build/", risk: "high" } });
});
await page.waitForTimeout(400);
const apv = page.locator(".call").filter({ hasText: "bash" }).last();
check("审批卡出现", await page.locator("text=/批准|允许|拒绝/").first().isVisible().catch(() => false));
await page.screenshot({ path: `${SHOTS}/5-审批卡.png` });

// 8) 会话树开得很大时：只列最近的，能展开全部，能折叠，行仍可点。
await page.goto(`${PAGE}?ws=6&sess=200`, { waitUntil: "networkidle" });
await page.waitForSelector(".wsnode", { timeout: 20000 });
await page.waitForTimeout(600);
const firstWs = page.locator(".wsnode").first();
check("大树只列最近的", (await firstWs.locator(".sessrow").count()) === 30, `${await firstWs.locator(".sessrow").count()} 行`);
check("有「全部显示」", await firstWs.locator("button.sessmore").isVisible());
await firstWs.locator("button.sessmore").click();
await page.waitForTimeout(400);
check("展开后列全", (await firstWs.locator(".sessrow").count()) === 200, `${await firstWs.locator(".sessrow").count()} 行`);
await firstWs.locator("button.twist").click();
await page.waitForTimeout(300);
check("折叠后收起", (await firstWs.locator(".sessrow").count()) === 0);
await page.screenshot({ path: `${SHOTS}/6-大会话树.png` });
const second = page.locator(".wsnode").nth(1).locator(".sessrow").first();
await second.click();
await page.waitForTimeout(800);
check("会话行可点开", (await page.locator(".pane").count()) >= 1, `${await page.locator(".pane").count()} 个面板`);

await browser.close();
console.log(fails.length ? `\n失败 ${fails.length} 项：\n- ${fails.join("\n- ")}` : "\n全部通过");
process.exit(fails.length ? 1 : 0);
