"use strict";
// Drives the agent's browser end to end in the real shell: a scripted model asks
// the real kernel to open and operate a page, and the checks read what the
// window actually drew. Run with `pnpm browser-live`; it needs no network.
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");
const { app } = require("electron");
const { checker, listen, scriptedModel, seedHome, settledWindow, until } = require("./livekit");

const PAGE = `<!doctype html><title>Greeter</title>
<label>Name <input></label>
<button onclick="document.querySelector('p').textContent='Hello, '+document.querySelector('input').value">Greet</button>
<p></p>`;

const { check, failures } = checker();

// Each round's call is built from what the previous tool result showed.
function browsingModel(pageURL) {
  return scriptedModel((tools) => {
    const last = tools.length ? tools[tools.length - 1] : "";
    const ref = (role, name) => (last.match(new RegExp(`- ${role} "${name}" \\[(e\\d+)\\]`)) || [])[1];
    if (tools.length === 0) return { name: "browser_open", arguments: { url: pageURL } };
    if (tools.length === 1) {
      return { name: "browser_act", arguments: { steps: [
        { action: "fill", ref: ref("textbox", "Name"), text: "李雷" },
        { action: "click", ref: ref("button", "Greet") },
      ] } };
    }
    return null;
  });
}

async function main() {
  const page = await listen((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(PAGE);
  });
  const { handler, requests } = browsingModel(`${page.url}/`);
  const model = await listen(handler);
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "rx-browser-live-"));
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), "rx-browser-ws-"));
  process.env.REASONIX_HOME = home;
  seedHome(home, model.url, workspace);
  const { current } = require("../src/main.js");

  try {
    const win = await settledWindow();
    const { client } = current();
    const runtimes = await until("a pane", async () => {
      const list = await client.json("GET", "/runtimes");
      return Array.isArray(list) && list.length ? list : null;
    });
    const base = runtimes[0].base;
    await client.request("POST", `${base}/submit`, { input: "greet 李雷 on the page" });
    await until("the scripted turn", async () => requests.length >= 3, 60000);

    const guests = () => win.contentView.children.filter((v) => v.webContents && v.webContents !== win.webContents);
    const guest = await until("a page view", async () => guests()[0]);
    check("the agent's page is a view in this window", guests().length === 1, guests().length);
    check("the view is not on the window's own session", guest.webContents.session !== win.webContents.session);
    check("the page loaded what the agent opened", guest.webContents.getURL() === `${page.url}/`, guest.webContents.getURL());
    const said = await guest.webContents.executeJavaScript("document.querySelector('p').textContent");
    check("the agent's input reached the page", said === "Hello, 李雷", said);
    const actResult = String((requests[2].messages || []).filter((m) => m.role === "tool").at(-1)?.content || "");
    check("the model was told what changed", actResult.includes('+ - text: "Hello, 李雷"'), actResult.slice(0, 400));

    const tabs = await client.json("GET", `${base}/browser/tabs`);
    check("the pane lists the tab with its view's target", tabs?.length === 1 && tabs[0].url === `${page.url}/`, tabs);
    const beneath = () => guest.getBounds().x < 0;
    check("while the panel is closed the page is not drawn", beneath());

    const js = (src) => win.webContents.executeJavaScript(src);
    await until("the browser tab in the pane bar", () => js(`!!document.querySelector('[data-action="pane.view"][data-value="browser"]')`));
    await js(`document.querySelector('[data-action="pane.view"][data-value="browser"]').click()`);
    const slot = await until("the reserved rectangle", async () => {
      const r = await js(`(() => { const e = document.querySelector('.bview'); if (!e) return null; const b = e.getBoundingClientRect(); return { x: b.left, y: b.top, width: b.width, height: b.height }; })()`);
      return r && r.width > 0 ? r : null;
    });
    const placed = await until("the page drawn over it", async () => (!beneath() ? guest.getBounds() : null));
    const close = (a, b) => Math.abs(a - b) <= 1;
    check("the page is drawn exactly over the rectangle the panel reserved",
      close(placed.x, slot.x) && close(placed.y, slot.y) && close(placed.width, slot.width) && close(placed.height, slot.height),
      { placed, slot });

    const shot = path.join(os.tmpdir(), "rx-browser-live.png");
    try {
      const id = win.getMediaSourceId().split(":")[1];
      execFileSync("screencapture", ["-x", "-o", "-l", id, shot]);
      process.stdout.write(`  screenshot: ${shot}\n`);
    } catch (err) {
      process.stdout.write(`  (no screenshot: ${err.message})\n`);
    }

    await js(`document.querySelector('[data-action="pane.view"][data-value="flow"]').click()`);
    await until("the page put away again", async () => beneath());
    check("leaving the browser view stops drawing the page", beneath());
  } catch (err) {
    check("the run completed", false, err.message);
  }
  process.stdout.write(failures.length ? `${failures.length} check(s) failed\n` : "all checks passed\n");
  app.exit(failures.length ? 1 : 0);
}

app.whenReady().then(() => setTimeout(main, 0));
