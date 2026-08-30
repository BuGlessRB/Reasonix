"use strict";
const { app, BrowserWindow, dialog, ipcMain, screen, session, shell } = require("electron");
const fs = require("node:fs/promises");
const path = require("node:path");
const { start } = require("./host");
const { StudioHost } = require("./hostclient");
const { installTray } = require("./tray");
const { installApplicationMenu, installContextMenu } = require("./menu");
const { externalTarget } = require("./links");

// Must match serve.TokenCookie and the namespace the kernel serves the page on.
const TOKEN_COOKIE = "reasonix_token";
const PAGE_PATH = "/_studio/";
const DEFAULT_SIZE = { width: 1440, height: 900 };
const MIN_SIZE = { width: 760, height: 480 };

const hostBinary =
  process.env.REASONIX_STUDIO_HOST || path.join(__dirname, "..", "bin", "reasonix-studio-host");
const pageDir =
  process.env.REASONIX_STUDIO_PAGE || path.join(__dirname, "..", "..", "frontend-next", "dist");

let kernel = null;
let win = null;
let origin = "";
let quitting = false;
let client = null;
let tray = null;

async function boot() {
  kernel = start(hostBinary, ["-page", pageDir], {
    onStderr: (text) => process.stderr.write(text),
    onExit: (code) => {
      if (code !== 0 && !quitting) app.quit();
    },
  });
  const ready = await kernel.ready;
  origin = ready.origin;
  client = new StudioHost(ready.origin, ready.token);
  await armCredential(ready);
  win = createWindow();
  guard(win.webContents);
  installContextMenu(win.webContents, win);
  win.once("ready-to-show", () => win.show());
  // No icon, no backgrounding: the close button can only hide the window where
  // something is left that brings it back.
  tray = installTray(client, { onOpen: showWindow, onQuit: () => app.quit() });
  win.on("close", onWindowClose);
  await win.loadURL(origin + PAGE_PATH);
  await tray?.refresh();
}

// Set before anything is loaded, or the first request answers 403 and the
// window opens on a refusal. HttpOnly because nothing in the page ever reads
// it, Strict because no cross-site navigation ever needs to carry it, and no
// expiry so it dies with this session rather than outliving the launch.
async function armCredential(ready) {
  await session.defaultSession.cookies.set({
    url: ready.origin + "/",
    name: TOKEN_COOKIE,
    value: ready.token,
    path: "/",
    httpOnly: true,
    secure: false,
    sameSite: "strict",
  });
}

// The close button hides where an icon can bring the window back, and quits
// where it cannot. Read from the cached preference rather than asked for on the
// spot: a close must not wait on the kernel, nor fail once it has gone.
function onWindowClose(event) {
  if (quitting || !tray?.prefs()?.closeToTray) return;
  event.preventDefault();
  win.hide();
}

// showWindow brings it back from wherever it went. Show alone is a no-op on a
// window that is merely buried, so the focus is what actually raises it.
function showWindow() {
  if (!win || win.isDestroyed()) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
}

// A window larger than the display it opens on puts its own title bar off
// screen, and a frameless one takes the only way to move it along with it.
function fitted() {
  const area = screen.getPrimaryDisplay().workAreaSize;
  return {
    width: Math.max(MIN_SIZE.width, Math.min(DEFAULT_SIZE.width, area.width)),
    height: Math.max(MIN_SIZE.height, Math.min(DEFAULT_SIZE.height, area.height)),
  };
}

function createWindow() {
  const mac = process.platform === "darwin";
  const windows = process.platform === "win32";
  const chrome = mac || windows;
  return new BrowserWindow({
    ...fitted(),
    minWidth: MIN_SIZE.width,
    minHeight: MIN_SIZE.height,
    // Shown once it has been measured against the screen it landed on; sizing a
    // visible window makes the correction a flicker.
    show: false,
    frame: !windows,
    titleBarStyle: mac ? "hiddenInset" : "default",
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      // Stated rather than left to the defaults: this is the list a reviewer
      // reads to know what the renderer may reach.
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
      webviewTag: false,
      webSecurity: true,
      additionalArguments: [`--reasonix-titlebar=${chrome ? "1" : "0"}`],
    },
  });
}

// The page may only ever be the origin the kernel handed us. A navigation
// anywhere else is refused rather than followed, and a second window is never
// opened at all — a link leaves through the platform opener or not at all.
function guard(contents) {
  contents.on("will-navigate", (event, url) => {
    if (!url.startsWith(origin + "/")) event.preventDefault();
  });
  contents.setWindowOpenHandler(() => ({ action: "deny" }));
}

// Every handler answers the one window this launch owns. A sender that is not
// its contents is refused rather than served.
function fromWindow(event) {
  return win && !win.isDestroyed() && event.sender === win.webContents ? win : null;
}

ipcMain.handle("window:minimise", (event) => {
  fromWindow(event)?.minimize();
});
ipcMain.handle("window:toggle-maximise", (event) => {
  const target = fromWindow(event);
  if (!target) return;
  if (target.isMaximized()) target.unmaximize();
  else target.maximize();
});
ipcMain.handle("window:is-maximised", (event) => fromWindow(event)?.isMaximized() ?? false);
ipcMain.handle("window:close", (event) => {
  fromWindow(event)?.close();
});
ipcMain.handle("shell:open-external", (event, raw) => {
  if (!fromWindow(event)) return;
  const target = externalTarget(raw);
  if (target) void shell.openExternal(target);
});

// A dismissed dialog answers with "", which is what the page reads as "they
// said no". Only a failure to write is an error.
async function saveTo(event, name, write) {
  const target = fromWindow(event);
  if (!target) return "";
  const picked = await dialog.showSaveDialog(target, { defaultPath: name });
  if (picked.canceled || !picked.filePath) return "";
  await write(picked.filePath);
  return picked.filePath;
}

ipcMain.handle("dialog:save-text", (event, name, content) =>
  saveTo(event, name, (path) => fs.writeFile(path, content, "utf8")),
);
ipcMain.handle("dialog:save-bytes", (event, name, bytes) =>
  saveTo(event, name, (path) => fs.writeFile(path, Buffer.from(bytes))),
);

app.whenReady().then(() => {
  installApplicationMenu();
  boot().catch((err) => {
    console.error("reasonix-studio:", err.message);
    app.quit();
  });
});

app.on("window-all-closed", () => app.quit());

// Closing this end of the pipe is what tells the kernel to drain. Without it a
// session file is left being written by a process nobody is holding open.
// What this launch is holding, for a test that drives the real shell rather
// than a copy of it.
module.exports = { current: () => ({ win, tray, client, origin }) };

app.on("before-quit", () => {
  quitting = true;
  tray?.close();
  kernel?.child.stdin.end();
});
