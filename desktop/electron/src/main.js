"use strict";
const { app, BrowserWindow, ipcMain, screen, session, shell } = require("electron");
const path = require("node:path");
const { start } = require("./host");
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

async function boot() {
  kernel = start(hostBinary, ["-page", pageDir], {
    onStderr: (text) => process.stderr.write(text),
    onExit: (code) => {
      if (code !== 0 && !quitting) app.quit();
    },
  });
  const ready = await kernel.ready;
  origin = ready.origin;
  await armCredential(ready);
  win = createWindow();
  guard(win.webContents);
  installContextMenu(win.webContents, win);
  win.once("ready-to-show", () => win.show());
  await win.loadURL(origin + PAGE_PATH);
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
app.on("before-quit", () => {
  quitting = true;
  kernel?.child.stdin.end();
});
