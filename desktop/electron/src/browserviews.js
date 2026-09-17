"use strict";
const crypto = require("node:crypto");
const { WebContentsView, session } = require("electron");
const { guestNavigationAllowed, typedAddress } = require("./browserguard");

// The size a page lays out at while nobody is looking at it: the kernel drives
// pages whether or not the panel is open, and a page sized to nothing reads as
// a phone layout or not at all.
const AGENT_VIEWPORT = { width: 1280, height: 900 };

// BrowserViews owns the pages the agent's browser opens in this window. Each is
// a view on its own partition — never the session the window's credential
// lives in. A page nobody is watching is not hidden and not moved wholly off
// screen: Chromium lays out nothing outside the window, and a page it created
// while its view was hidden takes no input until shown. It is kept overlapping
// the window by one pixel in the bottom-left corner instead.
class BrowserViews {
  constructor({ win, kernelOrigin }) {
    this.win = win;
    this.kernelOrigin = kernelOrigin;
    this.entries = new Map();
    this.guarded = new Set();
    this.shown = "";
    win.on("resize", () => {
      for (const [id, entry] of this.entries) if (id !== this.shown) this.putAway(entry.view);
    });
  }

  // guard fixes what every page on a partition may do: ask for no permission,
  // download nothing, and reach nothing on the kernel's origin, from any frame.
  guard(partition) {
    if (this.guarded.has(partition)) return;
    this.guarded.add(partition);
    const ses = session.fromPartition("persist:" + partition);
    ses.setPermissionRequestHandler((_wc, _permission, done) => done(false));
    ses.setPermissionCheckHandler(() => false);
    ses.webRequest.onBeforeRequest((details, done) => {
      let cancel = false;
      try {
        cancel = new URL(details.url).origin === this.kernelOrigin;
      } catch {
        cancel = false;
      }
      done({ cancel });
    });
    ses.on("will-download", (event, item, contents) => {
      event.preventDefault();
      for (const entry of this.entries.values()) {
        if (entry.view.webContents === contents) {
          entry.onDownload({ url: item.getURL(), suggestedFilename: item.getFilename() });
        }
      }
    });
  }

  create({ partition, url, onEvent, onClosed, onPopup, onDownload }) {
    this.guard(partition);
    const targetId = crypto.randomUUID();
    const view = new WebContentsView({
      webPreferences: {
        partition: "persist:" + partition,
        sandbox: true,
        contextIsolation: true,
        nodeIntegration: false,
        webviewTag: false,
        webSecurity: true,
        backgroundThrottling: false,
      },
    });
    const contents = view.webContents;
    let mainFrame = "";
    const allowed = (to) => guestNavigationAllowed(to, this.kernelOrigin);
    contents.on("will-navigate", (event, to) => {
      if (!allowed(to)) event.preventDefault();
    });
    contents.on("will-redirect", (event, to) => {
      if (!allowed(to)) event.preventDefault();
    });
    contents.setWindowOpenHandler(({ url: to }) => {
      if (allowed(to)) onPopup(to);
      return { action: "deny" };
    });
    contents.debugger.attach("1.3");
    contents.debugger.on("message", (_event, method, params, sessionId) => {
      if (sessionId) return;
      if (method === "Page.frameNavigated" && params && params.frame && !params.frame.parentId) {
        mainFrame = params.frame.id;
      }
      onEvent(method, params);
    });
    const entry = { view, onDownload };
    let closed = false;
    const close = () => {
      if (closed) return;
      closed = true;
      this.entries.delete(targetId);
      if (!this.win.isDestroyed()) this.win.contentView.removeChildView(view);
      if (!contents.isDestroyed()) contents.close();
      onClosed();
    };
    contents.once("destroyed", close);
    contents.on("render-process-gone", close);
    this.entries.set(targetId, entry);
    this.win.contentView.addChildView(view);
    this.putAway(view);
    void contents.loadURL(url).catch(() => {});
    return {
      targetId,
      send: (method, params) => contents.debugger.sendCommand(method, params),
      close,
      mainFrame: () => mainFrame,
    };
  }

  // show draws one page over the rectangle the page reserved for it, in window
  // coordinates, and puts every other page away.
  show(targetId, rect) {
    const entry = this.entries.get(targetId);
    if (!entry || !validRect(rect)) return this.hide();
    for (const [id, other] of this.entries) {
      if (id !== targetId) this.putAway(other.view);
    }
    this.shown = targetId;
    this.win.contentView.addChildView(entry.view);
    entry.view.setBounds({
      x: Math.round(rect.x),
      y: Math.round(rect.y),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
    });
  }

  hide() {
    this.shown = "";
    for (const entry of this.entries.values()) this.putAway(entry.view);
  }

  putAway(view) {
    const { height } = this.win.getContentBounds();
    view.setBounds({ x: 1 - AGENT_VIEWPORT.width, y: height - 1, ...AGENT_VIEWPORT });
  }

  control(targetId, action) {
    const contents = this.entries.get(targetId)?.view.webContents;
    if (!contents) return;
    const history = contents.navigationHistory;
    switch (action) {
      case "back":
        if (history.canGoBack()) history.goBack();
        break;
      case "forward":
        if (history.canGoForward()) history.goForward();
        break;
      case "reload":
        contents.reload();
        break;
      case "stop":
        contents.stop();
        break;
    }
  }

  navigate(targetId, address) {
    const contents = this.entries.get(targetId)?.view.webContents;
    const to = typedAddress(address);
    if (!contents || !guestNavigationAllowed(to, this.kernelOrigin)) return false;
    void contents.loadURL(to).catch(() => {});
    return true;
  }

  closeAll() {
    for (const entry of [...this.entries.values()]) entry.view.webContents.close();
  }
}

function validRect(rect) {
  return rect && [rect.x, rect.y, rect.width, rect.height].every(Number.isFinite) && rect.width > 0 && rect.height > 0;
}

module.exports = { BrowserViews, AGENT_VIEWPORT };
