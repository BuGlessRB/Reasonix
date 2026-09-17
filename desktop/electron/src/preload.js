"use strict";
const { contextBridge, ipcRenderer, webUtils } = require("electron");

// Whether the page draws the title bar is decided where the window is created;
// a sandboxed preload cannot require that module, so it is passed in.
const titleBar = process.argv.includes("--reasonix-titlebar=1");

function rectOf(rect) {
  return { x: Number(rect?.x), y: Number(rect?.y), width: Number(rect?.width), height: Number(rect?.height) };
}

// Verbs, and the two facts a layout needs. The origin the page was loaded from
// is already its own; the credential that opens it never crosses here at all.
contextBridge.exposeInMainWorld("reasonixHost", {
  shell: "electron",
  platform: process.platform,
  titleBar,
  minimiseWindow: () => ipcRenderer.invoke("window:minimise"),
  toggleMaximiseWindow: () => ipcRenderer.invoke("window:toggle-maximise"),
  isWindowMaximised: () => ipcRenderer.invoke("window:is-maximised"),
  closeWindow: () => ipcRenderer.invoke("window:close"),
  openExternal: (url) => ipcRenderer.invoke("shell:open-external", String(url)),
  // Where a dropped file lives. Resolved here rather than in the page: the
  // renderer is handed a File and never a path, and a turn that has to work on
  // the file itself cannot do it on a copy of the bytes.
  pathForFile: (file) => {
    try {
      return webUtils.getPathForFile(file);
    } catch {
      return "";
    }
  },
  saveText: (name, content) => ipcRenderer.invoke("dialog:save-text", String(name), String(content)),
  saveBytes: (name, bytes) => ipcRenderer.invoke("dialog:save-bytes", String(name), bytes),
  pickFolder: (startIn) => ipcRenderer.invoke("dialog:pick-folder", String(startIn)),
  // The agent's browser: which of its pages to draw over a rectangle of this
  // page, and the few controls a person has over a page they are watching.
  showBrowserView: (targetId, rect) => ipcRenderer.invoke("browser:show", String(targetId), rectOf(rect)),
  hideBrowserView: () => ipcRenderer.invoke("browser:hide"),
  controlBrowserView: (targetId, action) => ipcRenderer.invoke("browser:control", String(targetId), String(action)),
  navigateBrowserView: (targetId, address) => ipcRenderer.invoke("browser:navigate", String(targetId), String(address)),
});
