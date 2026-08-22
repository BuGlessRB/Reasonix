// Reports Chinese still rendered by an English interface.
//
// A catalogue's coverage cannot be decided from where a string is written: a
// constant table holding Chinese is correct when its render site calls t(),
// and `t(variable)` hides its key from every static check there is. What is on
// screen is the only judge, so this walks the real interface and reads it.
//
// Run it with a kernel and the dev server up, and a headless Chrome on 9333:
//
//   /path/to/reasonix serve -addr 127.0.0.1:8791 -auth none
//   REASONIX_SERVE=http://127.0.0.1:8791 npx vite --port 5177 --strictPort
//   chrome-headless-shell --remote-debugging-port=9333 --headless
//   node tools/i18n-sweep.mjs http://localhost:5177/
//
// Point REASONIX_HOME at an empty directory holding only a config.toml with a
// provider: a home with real sessions reports their titles, which are the
// user's words and not the interface's.
const base = "http://127.0.0.1:9333";
const url = process.argv[2];

let id = 0;
const pending = new Map();
const send = (ws, method, params = {}) => {
  const msgId = ++id;
  ws.send(JSON.stringify({ id: msgId, method, params }));
  return new Promise((r) => pending.set(msgId, r));
};

const list = await (await fetch(base + "/json/list")).json();
const target = list.find((t) => t.type === "page");
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m.result); pending.delete(m.id); }
};

const evaluate = async (expr) => {
  const r = await send(ws, "Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
  return r?.result?.value;
};

await send(ws, "Runtime.enable");
await send(ws, "Page.enable");
await send(ws, "Emulation.setDeviceMetricsOverride", { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
// The interface language is stored locally and fixed at boot, so set it first.
await send(ws, "Page.navigate", { url });
await new Promise((r) => setTimeout(r, 3000));
await evaluate(`localStorage.setItem("rx-lang", "en")`);
await send(ws, "Page.navigate", { url });
await new Promise((r) => setTimeout(r, 4000));

const HAN = "[\\u4e00-\\u9fff]";
const scan = (label) => evaluate(`(() => {
  const out = [];
  const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  let n; while ((n = walk.nextNode())) {
    const s = (n.textContent || "").trim();
    if (s && new RegExp("${HAN}").test(s)) out.push(s.slice(0, 60));
  }
  for (const el of document.querySelectorAll("[title],[aria-label],[placeholder]")) {
    for (const a of ["title", "aria-label", "placeholder"]) {
      const v = el.getAttribute(a);
      if (v && new RegExp("${HAN}").test(v)) out.push(a + "=" + v.slice(0, 50));
    }
  }
  return [...new Set(out)];
})()`).then((r) => ({ label, hits: r || [] }));

const results = [];
results.push(await scan("main"));
await evaluate(`document.querySelector('.thbtn[aria-label="Settings"]')?.click()`);
await new Promise((r) => setTimeout(r, 1500));
const count = await evaluate(`document.querySelectorAll('.prefs-nav button').length`);
if (!count) throw new Error("settings did not open; the sweep would prove nothing");
for (let i = 0; i < count; i++) {
  const name = await evaluate(`(() => {
    const b = document.querySelectorAll('.prefs-nav button')[${i}];
    b.click();
    return b.id || b.textContent.trim().slice(0, 12);
  })()`);
  await new Promise((r) => setTimeout(r, 1000));
  results.push(await scan(name));
}

let total = 0;
for (const { label, hits } of results) {
  if (!hits.length) continue;
  total += hits.length;
  console.log(`\n[${label}] ${hits.length}`);
  hits.slice(0, 12).forEach((h) => console.log("   " + h));
}
console.log(`\nsections walked: ${results.length}; Chinese still rendered: ${total}`);
ws.close();
