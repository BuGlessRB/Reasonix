"use strict";

// The context menu's shape, kept apart from the platform so it can be checked
// without one. editFlags is the page's own account of what is possible at the
// click: roles do the clipboard work, this only decides what to offer.
function contextTemplate(params) {
  if (!params.isEditable && !params.selectionText) return [];
  const can = params.editFlags ?? {};
  return [
    { role: "undo", enabled: !!can.canUndo },
    { role: "redo", enabled: !!can.canRedo },
    { type: "separator" },
    { role: "cut", enabled: !!can.canCut },
    { role: "copy", enabled: !!can.canCopy },
    { role: "paste", enabled: !!can.canPaste },
    { type: "separator" },
    { role: "selectAll", enabled: !!can.canSelectAll },
  ];
}

module.exports = { contextTemplate };
