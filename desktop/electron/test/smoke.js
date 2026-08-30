"use strict";
// Drives the real shell: the assertions below all run against the window
// src/main.js opened, loaded from the kernel it spawned. Run with `pnpm smoke`.
const { app, BrowserWindow, Menu } = require("electron");
require("../src/main.js");

const WINDOW_TIMEOUT_MS = 40000;
const failures = [];
const wait = (ms) => new Promise((r) => setTimeout(r, ms));

function check(name, condition, detail) {
  if (condition) {
    process.stdout.write(`  ok   ${name}\n`);
    return;
  }
  failures.push(name);
  process.stdout.write(`  FAIL ${name}${detail === undefined ? "" : ` — ${JSON.stringify(detail)}`}\n`);
}

async function settledWindow() {
  const deadline = Date.now() + WINDOW_TIMEOUT_MS;
  while (Date.now() < deadline) {
    const win = BrowserWindow.getAllWindows()[0];
    if (win && !win.webContents.isLoading() && win.webContents.getURL()) return win;
    await wait(200);
  }
  throw new Error("the shell never opened a loaded window");
}

async function run() {
  const win = await settledWindow();
  const js = (src) => win.webContents.executeJavaScript(src);
  const url = new URL(win.webContents.getURL());

  check("the page is loaded from the loopback kernel", url.hostname === "127.0.0.1" && url.protocol === "http:", url.href);
  check("the page is loaded from its own namespace", url.pathname === "/_studio/", url.pathname);

  const seen = await js(`(() => ({
    bridge: window.reasonixHost ? Object.keys(window.reasonixHost).sort() : null,
    cookie: document.cookie,
    globals: [typeof require, typeof process, typeof module, typeof Buffer],
    shell: document.documentElement.dataset.shell,
    platform: document.documentElement.dataset.platform,
    titlebar: document.documentElement.dataset.titlebar,
    mounted: (document.getElementById('root')?.children.length ?? 0) > 0,
  }))()`);

  const verbs = ["closeWindow", "isWindowMaximised", "minimiseWindow", "openExternal", "platform", "shell", "titleBar", "toggleMaximiseWindow"];
  check("the bridge exposes verbs and nothing else", JSON.stringify(seen.bridge) === JSON.stringify(verbs), seen.bridge);
  check("the credential never reaches the page", !seen.cookie.includes("reasonix_token"), seen.cookie);
  check("the renderer has no node of its own", seen.globals.every((t) => t === "undefined"), seen.globals);
  check("the page knows which shell it is in", seen.shell === "electron", seen.shell);
  check("the page knows the window draws its own title bar", seen.titlebar === "app", seen.titlebar);
  check("the page knows the platform", ["darwin", "windows", "linux"].includes(seen.platform), seen.platform);
  check("the app actually mounted", seen.mounted);

  // The whole boundary in one request: same origin, HttpOnly cookie, and a
  // kernel that answers with JSON rather than the page.
  const status = await js(`fetch('/status', { credentials: 'same-origin' })
    .then((r) => ({ code: r.status, type: r.headers.get('content-type') ?? '' }))
    .catch((e) => ({ code: -1, type: String(e) }))`);
  check("the page reaches the kernel through the gate", status.code === 200 && status.type.includes("json"), status);

  const prefs = win.webContents.getLastWebPreferences() ?? {};
  check("node integration is off", prefs.nodeIntegration !== true);
  check("context isolation is on", prefs.contextIsolation !== false);
  check("the renderer is sandboxed", prefs.sandbox === true);
  check("webview tags are off", prefs.webviewTag !== true);
  check("web security is on", prefs.webSecurity !== false);

  const bounds = win.getBounds();
  const [minWidth, minHeight] = win.getMinimumSize();
  const area = require("electron").screen.getPrimaryDisplay().workAreaSize;
  check("the window fits the display it opened on", bounds.width <= area.width && bounds.height <= area.height, { bounds, area });
  check("the window cannot be shrunk past its layout", minWidth === 760 && minHeight === 480, { minWidth, minHeight });

  // A window that cannot be navigated away from, and cannot open another.
  let prevented = false;
  win.webContents.emit("will-navigate", { preventDefault: () => (prevented = true) }, "https://attacker.example/");
  check("a navigation off this origin is refused", prevented);
  prevented = false;
  win.webContents.emit("will-navigate", { preventDefault: () => (prevented = true) }, url.origin + "/_studio/x");
  check("a navigation inside this origin is allowed", !prevented);
  await js(`window.open('https://attacker.example/', '_blank')`).catch(() => {});
  await wait(400);
  check("a second window is never opened", BrowserWindow.getAllWindows().length === 1, BrowserWindow.getAllWindows().length);

  if (process.platform === "darwin") await menuChecks(win, js);
  check("the context menu is wired to the window", win.webContents.listenerCount("context-menu") > 0);
}

// The failure this guards is a window whose editing shortcuts do nothing at
// all, which is what a WebContents without an application menu has. What can be
// asserted here is that the menu exists carrying the roles the platform routes
// through, and that the window's editing pipeline is live. The last hop —
// AppKit turning a key equivalent into that role — is not reachable from a
// test: MenuItem.click() runs the JS handler a role does not have, and
// sendInputEvent injects below the dispatch that consults the menu. It was
// accepted by hand instead, at the OS event layer.
async function menuChecks(win, js) {
  const menu = Menu.getApplicationMenu();
  const roles = (menu?.items ?? []).map((i) => String(i.role).toLowerCase());
  check("the application menu is installed", !!menu);
  check("it carries the three menus macOS routes through", ["appmenu", "editmenu", "windowmenu"].every((r) => roles.includes(r)), roles);

  const edit = (menu?.items ?? []).find((i) => String(i.role).toLowerCase() === "editmenu");
  const editRoles = (edit?.submenu?.items ?? []).map((i) => String(i.role).toLowerCase());
  check("the edit menu carries every editing command", ["undo", "redo", "cut", "copy", "paste", "selectall"].every((r) => editRoles.includes(r)), editRoles);

  app.focus({ steal: true });
  win.focus();
  await wait(500);
  await js(`(() => {
    let probe = document.getElementById('smoke-probe');
    if (!probe) {
      probe = document.createElement('textarea');
      probe.id = 'smoke-probe';
      document.body.appendChild(probe);
    }
    probe.value = 'HELLO';
    probe.focus();
    probe.setSelectionRange(0, 0);
  })()`);
  await wait(200);
  // An editing command issued at the contents lands on the focused field. A
  // window that failed this could not be edited by any route, menu or not.
  win.webContents.selectAll();
  await wait(400);
  const reached = await js(`(() => {
    const p = document.getElementById('smoke-probe');
    return { start: p.selectionStart, end: p.selectionEnd };
  })()`);
  check("an editing command reaches the focused field", reached.start === 0 && reached.end === 5, reached);
  await js(`document.getElementById('smoke-probe')?.remove()`);
  process.stdout.write("  note the key-equivalent hop (Cmd-C reaching the copy role) is not reachable\n");
  process.stdout.write("  note from a test; it was accepted by hand at the OS event layer.\n");
}

app.whenReady().then(async () => {
  process.stdout.write("reasonix studio shell — smoke\n");
  try {
    await run();
  } catch (err) {
    failures.push(String(err && err.message));
    process.stdout.write(`  FAIL ${String(err && err.message)}\n`);
  }
  process.stdout.write(failures.length ? `\n${failures.length} failed\n` : "\nall checks passed\n");
  process.exit(failures.length ? 1 : 0);
});
