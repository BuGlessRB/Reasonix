// 补全菜单的选中态：键盘走到哪一行，那一行必须是画面上唯一亮着的。
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const BOX = 'textarea[role="combobox"]';

// 选中要压过悬停,而且要往同一个方向压 —— 深色下抬亮、浅色下压暗。方向搞反
// 的填色（把 --accent-wash 直接搬过来就是）在浅色下看着还行,深色下是个洞。
const OVER = 1.4;
const FLOOR = 8;

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

const read = () =>
  page.evaluate(() => {
    const list = document.querySelector("#slashmenu");
    if (!list) return null;
    const panel = list.closest(".slashmenu");
    // 十六进制来自令牌,color() 来自 color-mix,rgb() 来自其它一切。
    const rgb = (s) => {
      s = s.trim();
      if (s.startsWith("#")) {
        const h = s.length === 4 ? [...s.slice(1)].map((c) => c + c).join("") : s.slice(1);
        return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16)).concat(1);
      }
      const n = (s.match(/-?[\d.]+/g) ?? []).map(Number);
      const [r = 0, g = 0, b = 0, a = 1] = n;
      return s.startsWith("color(") ? [r * 255, g * 255, b * 255, a] : [r, g, b, a];
    };
    const lum = ([r, g, b]) => 0.2126 * r + 0.7152 * g + 0.0722 * b;
    const st = getComputedStyle(panel);
    const floor = lum(rgb(st.backgroundColor));
    return {
      kb: list.hasAttribute("data-kb"),
      caret: document.querySelector('[role="combobox"]').getAttribute("aria-activedescendant"),
      // .mi:hover 画的就是这个令牌,所以读它等于读悬停态。
      hover: lum(rgb(st.getPropertyValue("--float-hi"))) - floor,
      rows: [...list.querySelectorAll("button.mi")].map((b) => {
        const s = getComputedStyle(b);
        const c = rgb(s.backgroundColor);
        return {
          on: b.hasAttribute("data-on"),
          fill: c[3] === 0 ? null : lum(c) - floor,
          rail: s.boxShadow !== "none",
        };
      }),
    };
  });

const park = async (i) => {
  const b = await page.locator("#slashmenu button.mi").nth(i).boundingBox();
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2);
  await page.waitForTimeout(80);
};

const key = async (k) => {
  await page.keyboard.press(k);
  await page.waitForTimeout(80);
};

async function suite(tag, token, park1, park2) {
  await page.click(BOX);
  await page.keyboard.press("ControlOrMeta+a");
  await page.keyboard.press("Backspace");
  await page.type(BOX, token, { delay: 20 });
  await page.waitForSelector("#slashmenu button.mi", { timeout: 5000 });
  await page.waitForTimeout(150);

  let s = await read();
  check(`${tag} 菜单开着,只有一行选中`, s && s.rows.filter((r) => r.on).length === 1, `${s?.rows.length} 行`);

  const on = s.rows.find((r) => r.on);
  const beat = on.fill !== null && Math.sign(on.fill) === Math.sign(s.hover) && Math.abs(on.fill) >= Math.abs(s.hover) * OVER;
  check(`${tag} 选中比悬停更重,方向一致`, beat && Math.abs(on.fill) >= FLOOR, `选中 ${on.fill?.toFixed(1)} / 悬停 ${s.hover.toFixed(1)}`);
  check(`${tag} 选中行有那条竖杠`, on.rail);

  await key("ArrowDown");
  s = await read();
  check(`${tag} 方向键把选中挪走了`, s.rows.findIndex((r) => r.on) === 1 && s.caret === "slash-1");

  // 指针停在别的行上,手再回到键盘 —— 这里曾经同时亮两行,
  // 而且更重的那一行是指针底下那一行,不是回车会拿走的那一行。
  await park(park1);
  s = await read();
  check(`${tag} 指针一动就接管`, !s.kb && s.rows.findIndex((r) => r.on) === park1);

  await key("ArrowDown");
  s = await read();
  const lit = s.rows.filter((r) => r.fill !== null);
  check(`${tag} 键盘接回来,指针那行不再亮`, s.kb && lit.length === 1 && lit[0].on, `亮着 ${lit.length} 行`);
  check(`${tag} 亮的是键盘走到的那一行`, s.rows.findIndex((r) => r.on) === park1 + 1);

  await park(park2);
  s = await read();
  check(`${tag} 指针再动,又归指针`, !s.kb && s.rows.findIndex((r) => r.on) === park2);
}

for (const scheme of ["light", "dark"]) {
  console.log(`\n${scheme}`);
  await page.emulateMedia({ colorScheme: scheme });
  await page.waitForTimeout(300);
  await suite(`${scheme}/斜杠`, "/", 3, 1);
  await suite(`${scheme}/引用`, "@", 2, 0);
}

await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} 项不合格:\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\n全部通过");
