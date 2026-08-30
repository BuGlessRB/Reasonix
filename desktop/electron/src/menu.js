"use strict";
const { Menu } = require("electron");
const { contextTemplate } = require("./editmenu");

// Without an application menu a WebContents has no copy, paste or undo at all:
// macOS routes those shortcuts through it, and a window with none reads as a
// broken text editor. Elsewhere the bar would render inside the window, and
// those platforms bind the shortcuts themselves.
function installApplicationMenu() {
  if (process.platform !== "darwin") {
    Menu.setApplicationMenu(null);
    return null;
  }
  const menu = Menu.buildFromTemplate([
    { role: "appMenu" },
    { role: "editMenu" },
    { role: "windowMenu" },
  ]);
  Menu.setApplicationMenu(menu);
  return menu;
}

// A production build ships without one, so a right-click in a text field offered
// nothing at all. Roles rather than handlers: the clipboard work belongs to the
// platform, and editFlags is the page's own account of what is possible here.
function installContextMenu(contents, window) {
  contents.on("context-menu", (_event, params) => {
    const template = contextTemplate(params);
    if (!template.length) return;
    Menu.buildFromTemplate(template).popup({ window });
  });
}

module.exports = { installApplicationMenu, installContextMenu };
