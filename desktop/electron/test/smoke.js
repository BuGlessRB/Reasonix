"use strict";
// Drives the real shell: the assertions below all run against the window
// src/main.js opened, loaded from the kernel it spawned. Run with `pnpm smoke`.
const os = require("node:os");
const path = require("node:path");
const { app, BrowserWindow, Menu } = require("electron");
const { current } = require("../src/main.js");

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
    placed: typeof window.reasonixHost?.pathForFile(new File(['x'], 'x.txt')),
  }))()`);

  const verbs = ["closeWindow", "isWindowMaximised", "minimiseWindow", "openExternal", "pathForFile", "platform", "saveBytes", "saveText", "shell", "titleBar", "toggleMaximiseWindow"];
  check("the bridge exposes verbs and nothing else", JSON.stringify(seen.bridge) === JSON.stringify(verbs), seen.bridge);
  check("the credential never reaches the page", !seen.cookie.includes("reasonix_token"), seen.cookie);
  check("the renderer has no node of its own", seen.globals.every((t) => t === "undefined"), seen.globals);
  check("the page knows which shell it is in", seen.shell === "electron", seen.shell);
  check("the page knows the window draws its own title bar", seen.titlebar === "app", seen.titlebar);
  check("the page knows the platform", ["darwin", "windows", "linux"].includes(seen.platform), seen.platform);
  check("the app actually mounted", seen.mounted);
  // A sandboxed preload can only reach part of Electron; a build where webUtils
  // is not part of it would have thrown before the bridge existed at all, and a
  // dropped file would have no path anywhere in the window.
  check("the shell can place a dropped file", seen.placed === "string", seen.placed);

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

  await dropChecks(win, js);
  await trayChecks(win);
  await instanceChecks(win);

  if (process.platform === "darwin") await menuChecks(win, js);
  check("the context menu is wired to the window", win.webContents.listenerCount("context-menu") > 0);
}

// The icon is main's surface, and main reaches the kernel for it directly: a
// tray that asked the page for its state would put the credential in reach of
// the page, and would go blank the moment the window was hidden.
async function trayChecks(win) {
  const { tray, client } = current();
  check("the status icon came up", !!tray);
  if (!tray) return;

  const prefs = await client.trayPrefs();
  check("main reads the tray settings from the kernel", !!prefs && typeof prefs.closeToTray === "boolean", prefs);
  const fold = await client.trayState();
  check("main reads the fold from the kernel", !!fold && typeof fold.mood === "string", fold);
  check("the fold arrives spelled out", !!fold?.line && !!fold?.labels?.quit, fold);

  // The icon shows the kernel's fold, not one this shell kept for itself. A
  // tray with its own counter would drift from what the panes actually did,
  // and nothing on screen would say which of the two was wrong.
  await tray.refresh();
  const fresh = await client.trayState();
  check("the icon shows the kernel's fold", tray.fold()?.line === fresh?.line, {
    icon: tray.fold()?.line,
    kernel: fresh?.line,
  });

  // Backgrounding, end to end: the setting is written through the kernel, the
  // window is closed, and it is still there — hidden rather than gone.
  const on = await client.setTrayPrefs(true, true);
  check("backgrounding can be turned on where an icon is up", on?.closeToTray === true, on);
  await tray.refresh();
  win.close();
  await wait(500);
  check("the close button hides the window rather than ending it", !win.isDestroyed() && !win.isVisible(), {
    destroyed: win.isDestroyed(),
    visible: win.isVisible(),
  });

  win.show();
  await wait(200);
  const off = await client.setTrayPrefs(true, false);
  check("backgrounding can be turned back off", off?.closeToTray === false, off);
  await tray.refresh();
}

// One data home is one Studio. A second launch over the same home must not
// start a second kernel on those session files; it must bring back the window
// that is already holding them, wherever it went.
async function instanceChecks(win) {
  const { execFile } = require("node:child_process");
  const appDir = path.join(__dirname, "..");

  const launch = (home) =>
    new Promise((resolve) => {
      const started = Date.now();
      const child = execFile(process.execPath, [appDir], { env: { ...process.env, REASONIX_HOME: home } }, () =>
        resolve({ took: Date.now() - started, child }),
      );
      // Anything still running after this is a launch that decided it was the
      // first one, which is the answer for a different home.
      setTimeout(() => resolve({ took: -1, child }), 8000);
    });

  win.hide();
  await wait(300);
  const same = await launch(process.env.REASONIX_HOME);
  check("a second launch over the same home leaves", same.took > 0, same.took);
  check("and raises the window that was already holding it", win.isVisible(), {
    visible: win.isVisible(),
  });

  // The kernel this window is holding is untouched by the launch that left: a
  // second one starting on these session files is the whole hazard.
  const { client } = current();
  check("the running kernel is undisturbed", !!(await client.trayPrefs()));

  const otherHome = path.join(os.tmpdir(), "rx-other-home");
  const elsewhere = await launch(otherHome);
  check("a launch over another home is allowed to run", elsewhere.took === -1, elsewhere.took);

  // Killed rather than asked to quit, which is what a crash looks like. The
  // lock is the platform's and goes with the process, so there is no file of
  // ours left holding a home nobody is in.
  elsewhere.child.kill("SIGKILL");
  await wait(1500);
  const after = await launch(otherHome);
  check("a home whose holder died can be opened again", after.took === -1, after.took);
  after.child.kill();
}

// A dropped file has to arrive as a path, or a turn works on a copy of the
// bytes instead of on the file. The drop is dispatched at the browser's own
// input layer carrying a real file, so what is measured is the whole chain:
// the event, the File the page is handed, and what the shell can place it at.
async function dropChecks(win, js) {
  const fsp = require("node:fs/promises");
  const file = path.join(await fsp.mkdtemp(path.join(os.tmpdir(), "rx-drop-")), "dropped.txt");
  await fsp.writeFile(file, "dropped");
  const before = win.webContents.getURL();

  await js(`(() => {
    window.__smokeDrop = null;
    addEventListener('drop', (e) => {
      const files = [...(e.dataTransfer?.files ?? [])];
      window.__smokeDrop = files.map((f) => window.reasonixHost.pathForFile(f)).filter(Boolean);
    }, true);
  })()`);

  const dbg = win.webContents.debugger;
  if (!dbg.isAttached()) dbg.attach("1.3");
  const data = { items: [], files: [file], dragOperationsMask: 1 };
  for (const type of ["dragEnter", "dragOver", "drop"]) {
    await dbg.sendCommand("Input.dispatchDragEvent", { type, x: 200, y: 200, data, modifiers: 0 });
    await wait(200);
  }
  await wait(400);
  const placed = await js(`window.__smokeDrop`);
  check("a dropped file arrives as a path", Array.isArray(placed) && placed[0] === file, placed);
  // The default this window must never take: navigating to what was dropped
  // replaces the app with the file, and nothing short of a reload comes back.
  check("a dropped file does not navigate the window", win.webContents.getURL() === before, win.webContents.getURL());
  dbg.detach();
  await fsp.rm(path.dirname(file), { recursive: true, force: true });
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
