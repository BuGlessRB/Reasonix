// Contrast of rendered text against what is actually behind it, walked over
// every settings section in both themes.
//
// The token table cannot answer this. A colour is checked against the surface
// it was designed for, then used on another one; a tint made with color-mix is
// translucent, so the real background is a composite that exists only at
// render time. This reads what the pixels resolve to.
//
// Run it with the same setup as i18n-sweep.mjs (kernel, dev server, headless
// Chrome on 9333), then:
//
//   node tools/contrast-sweep.mjs http://localhost:5177/
//
// Two things it deliberately does NOT do quietly: a probe that fails to
// evaluate throws rather than returning an empty list, because an empty list
// reads exactly like a clean sweep. And getComputedStyle returns
// `color(srgb r g b / a)` for color-mix results — 0-1 channels, not 0-255 —
// which parsed as 8-bit turns a pale tint into a mid grey.
const list = await (await fetch("http://127.0.0.1:9333/json/list")).json();
const t = list.find((x) => x.type === "page");
const ws = new WebSocket(t.webSocketDebuggerUrl);
let id = 0; const pend = new Map();
const send = (m, p = {}) => { const i = ++id; ws.send(JSON.stringify({ id: i, method: m, params: p })); return new Promise((r) => pend.set(i, r)); };
await new Promise((r) => (ws.onopen = r));
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && pend.has(m.id)) { pend.get(m.id)(m.result); pend.delete(m.id); } };
const ev = async (x) => (await send("Runtime.evaluate", { expression: x, returnByValue: true, awaitPromise: true }))?.result?.value;
await send("Runtime.enable"); await send("Page.enable");
await send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false });

const probe = `(() => {
  const lum = (c) => {
    const [r, g, b] = c.map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const parse = (s) => {
    const nums = (s.match(/[\\d.]+/g) || []).map(Number);
    if (/^color\\(/.test(s)) {
      const [r, g, b, a] = nums;
      return a === undefined ? [r * 255, g * 255, b * 255] : [r * 255, g * 255, b * 255, a];
    }
    return nums.slice(0, 4);
  };
  const over = (fg, bg) => {
    const a = fg.length > 3 ? fg[3] : 1;
    return [0, 1, 2].map((i) => fg[i] * a + bg[i] * (1 - a));
  };
  const bgOf = (el) => {
    const stack = [];
    let cur = el;
    while (cur && cur !== document.documentElement) {
      const c = parse(getComputedStyle(cur).backgroundColor);
      if (c.length >= 3 && (c.length < 4 || c[3] > 0)) {
        stack.push(c);
        if (c.length < 4 || c[3] === 1) break;
      }
      cur = cur.parentElement;
    }
    let base = [255, 255, 255];
    for (let i = stack.length - 1; i >= 0; i--) base = over(stack[i], base);
    return base;
  };
  const seen = new Map();
  for (const el of document.querySelectorAll("body *")) {
    const own = [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent.trim());
    if (!own) continue;
    const s = getComputedStyle(el);
    if (s.visibility === "hidden" || s.display === "none" || +s.opacity === 0) continue;
    const r = el.getBoundingClientRect();
    if (r.width < 2 || r.height < 2) continue;
    const size = parseFloat(s.fontSize), weight = +s.fontWeight || 400;
    const fg = parse(s.color), bg = bgOf(el);
    const blended = fg.length > 3 ? over(fg, bg) : fg.slice(0, 3);
    const l1 = lum(blended), l2 = lum(bg);
    const ratio = (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
    // WCAG large-text exemption: 18.66px bold, or 24px.
    const large = size >= 24 || (size >= 18.66 && weight >= 700);
    const floor = large ? 3 : 4.5;
    if (ratio + 0.005 < floor) {
      const hex = (c) => "#" + c.slice(0, 3).map((v) => Math.round(v).toString(16).padStart(2, "0")).join("").toUpperCase();
      const key = hex(blended) + "|" + hex(bg);
      if (!seen.has(key)) seen.set(key, {
        ratio: +ratio.toFixed(2), floor, fg: hex(blended), bg: hex(bg),
        cls: (el.className || el.tagName).toString().slice(0, 22),
      });
    }
  }
  return [...seen.values()].sort((a, b) => a.ratio - b.ratio).slice(0, 14);
})()`;

for (const theme of ["light", "dark"]) {
  await send("Page.navigate", { url: process.argv[2] });
  await new Promise((r) => setTimeout(r, 2600));
  await ev(`document.documentElement.setAttribute("data-theme", ${JSON.stringify(theme)})`);
  await new Promise((r) => setTimeout(r, 600));
  const all = new Map();
  const take = async (label) => {
    const got = await ev(probe);
    if (!Array.isArray(got)) throw new Error("probe did not evaluate — a silent [] would read as a clean sweep");
    for (const r of got) all.set(r.fg + "|" + r.bg, { ...r, where: label });
  };
  await take("main");
  await ev(`document.querySelector('.thbtn[aria-label="Settings"], .thbtn[aria-label="设置"]')?.click()`);
  await new Promise((r) => setTimeout(r, 1400));
  const count = await ev(`document.querySelectorAll('.prefs-nav button').length`);
  for (let i = 0; i < count; i++) {
    const name = await ev(`(() => { const b = document.querySelectorAll('.prefs-nav button')[${i}]; b.click(); return b.id || String(${i}); })()`);
    await new Promise((r) => setTimeout(r, 850));
    await take(name);
  }
  const rows = [...all.values()].sort((a, b) => a.ratio - b.ratio);
  console.log(`\n=== ${theme} — ${count} sections — below AA: ${rows.length} ===`);
  for (const r of rows) console.log(`  ${String(r.ratio).padStart(5)} (need ${r.floor})  ${r.fg} on ${r.bg}   ${r.cls.padEnd(20)} @${r.where}`);
}
ws.close();
