"use strict";
// Drives the agent's browser end to end in the real shell: a scripted model asks
// the real kernel to open and operate a page, and the checks read what the
// window actually drew. Run with `pnpm browser-live`; it needs no network.
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs");
const http = require("node:http");
const { execFileSync } = require("node:child_process");
const { app, BrowserWindow } = require("electron");

const PAGE = `<!doctype html><title>Greeter</title>
<label>Name <input></label>
<button onclick="document.querySelector('p').textContent='Hello, '+document.querySelector('input').value">Greet</button>
<p></p>`;

const failures = [];
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
function check(name, condition, detail) {
  process.stdout.write(condition ? `  ok   ${name}\n` : `  FAIL ${name}${detail === undefined ? "" : ` — ${JSON.stringify(detail)}`}\n`);
  if (!condition) failures.push(name);
}

function listen(handler) {
  return new Promise((resolve) => {
    const server = http.createServer(handler);
    server.listen(0, "127.0.0.1", () => resolve({ server, url: `http://127.0.0.1:${server.address().port}` }));
  });
}

// The scripted model: each request answers with the next tool call, built from
// what the previous tool result showed it, then with a closing reply.
const requests = [];
function modelHandler(pageURL) {
  return (req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const parsed = JSON.parse(body || "{}");
      requests.push(parsed);
      const tools = (parsed.messages || []).filter((m) => m.role === "tool");
      const last = tools.length ? String(tools[tools.length - 1].content || "") : "";
      const ref = (role, name) => (last.match(new RegExp(`- ${role} "${name}" \\[(e\\d+)\\]`)) || [])[1];
      let call = null;
      if (tools.length === 0) call = { name: "browser_open", arguments: { url: pageURL } };
      else if (tools.length === 1) {
        call = { name: "browser_act", arguments: { steps: [
          { action: "fill", ref: ref("textbox", "Name"), text: "李雷" },
          { action: "click", ref: ref("button", "Greet") },
        ] } };
      }
      res.writeHead(200, { "Content-Type": "text/event-stream" });
      const chunk = (delta, finish) =>
        res.write(`data: ${JSON.stringify({ id: "c", object: "chat.completion.chunk", choices: [{ index: 0, delta, finish_reason: finish }] })}\n\n`);
      if (call) {
        chunk({ role: "assistant", tool_calls: [{ index: 0, id: `call-${tools.length}`, type: "function", function: { name: call.name, arguments: JSON.stringify(call.arguments) } }] }, null);
        chunk({}, "tool_calls");
      } else {
        chunk({ role: "assistant", content: "done" }, null);
        chunk({}, "stop");
      }
      res.write("data: [DONE]\n\n");
      res.end();
    });
  };
}

function seedHome(home, modelURL, workspace) {
  fs.mkdirSync(home, { recursive: true });
  fs.writeFileSync(path.join(home, "config.toml"), [
    'default_model = "fake/fake-model"',
    "",
    "[desktop]",
    "welcomed = true",
    'default_tool_approval_mode = "yolo"',
    "",
    "[[providers]]",
    'name        = "fake"',
    'kind        = "openai"',
    `base_url    = "${modelURL}/v1"`,
    'models      = ["fake-model"]',
    'default     = "fake-model"',
    'api_key_env = "FAKE_API_KEY"',
    "",
  ].join("\n"));
  fs.writeFileSync(path.join(home, ".env"), "FAKE_API_KEY=fake\n");
  process.chdir(workspace);
}

async function settledWindow() {
  const deadline = Date.now() + 40000;
  while (Date.now() < deadline) {
    const win = BrowserWindow.getAllWindows()[0];
    if (win && !win.webContents.isLoading() && win.webContents.getURL()) return win;
    await wait(200);
  }
  throw new Error("the shell never opened a loaded window");
}

async function until(what, fn, ms = 30000) {
  const deadline = Date.now() + ms;
  while (Date.now() < deadline) {
    const got = await fn();
    if (got) return got;
    await wait(200);
  }
  throw new Error(`timed out waiting for ${what}`);
}

async function main() {
  const page = await listen((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(PAGE);
  });
  const model = await listen(modelHandler(`${page.url}/`));
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
