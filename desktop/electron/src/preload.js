"use strict";
const { contextBridge, ipcRenderer, webUtils } = require("electron");

// Whether the page draws the title bar is decided where the window is created;
// a sandboxed preload cannot require that module, so it is passed in.
const titleBar = process.argv.includes("--reasonix-titlebar=1");

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
});
