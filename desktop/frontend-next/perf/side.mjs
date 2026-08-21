// 右栏（度量面板）：端点自己写的话有多长不由我们定，那一栏有多宽由我们定。
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";

const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1500, height: 900 } });
page.on("pageerror", (e) => fails.push("页面异常: " + e.message));

await page.goto(PAGE, { waitUntil: "networkidle" });
await page.waitForSelector(".app", { timeout: 20000 });
await page.waitForSelector('[data-b="mcp"] .srvrow', { timeout: 10000 });
await page.waitForTimeout(400);

const read = () =>
  page.evaluate(() => {
    const scroll = document.querySelector(".side .scroll");
    const row = document.querySelector('[data-b="mcp"] .srvrow');
    const nm = row.querySelector(".nm");
    const why = row.querySelector(".why");
    const rs = getComputedStyle(row);
    const ws = getComputedStyle(why);
    return {
      // 栏是固定宽的，一旦横向能滚，就是有东西把它撑开了 —— 整栏的每一块都跟着错位。
      overflow: scroll.scrollWidth - scroll.clientWidth,
      tag: row.tagName,
      chrome: `${rs.borderTopStyle} ${rs.backgroundColor}`,
      nmW: Math.round(nm.getBoundingClientRect().width),
      nmMono: getComputedStyle(nm).fontFamily.includes("mono") || getComputedStyle(nm).fontFamily.includes("Mono"),
      whyMono: ws.fontFamily.includes("mono") || ws.fontFamily.includes("Mono"),
      whyLines: Math.round(why.getBoundingClientRect().height / parseFloat(ws.lineHeight)),
      whyChars: why.textContent.length,
      bar: rs.boxShadow,
    };
  });

let s = await read();
check("端点报的错就是长的", s.whyChars > 80, `${s.whyChars} 字`);
check("这一栏没被撑开", s.overflow === 0, `横向溢出 ${s.overflow}px`);
check("名字没有被原因挤没", s.nmW > 0, `${s.nmW}px`);
check("原因夹在两行以内", s.whyLines > 0 && s.whyLines <= 2, `${s.whyLines} 行`);
// 借 .job 的样式时这里是 <button> 套着系统那圈 outset 边框和灰底，
// 而栏里其它每一行都是无边框的。
check("不是一枚系统按钮", s.tag === "BUTTON" && s.chrome === "none rgba(0, 0, 0, 0)", s.chrome);
check("连不上用左色条说", s.bar.includes("inset"), s.bar);
// 名字是机器事实，跟栏里其它行一样等宽；原因是一句话，等宽栈里没有汉字。
check("名字等宽、原因界面字", s.nmMono && !s.whyMono, `nm mono=${s.nmMono} why mono=${s.whyMono}`);

// 端点常把一整条 PATH 或 URL 塞进来，中间一个空格都没有。
await page.evaluate(() => {
  document.querySelector('[data-b="mcp"] .why').textContent =
    "https://example.com/" + "a".repeat(260) + "?token=deadbeef";
});
await page.waitForTimeout(250);
s = await read();
check("一整串没有空格的也断得开", s.overflow === 0, `横向溢出 ${s.overflow}px`);
check("断开之后仍然只占两行", s.whyLines <= 2, `${s.whyLines} 行`);

await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} 项不合格:\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\n全部通过");
