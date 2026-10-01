"use strict";

// Role labels come from the platform's own catalogue, which follows the
// system locale rather than the language the interface was set to, so they are
// stated here for the two languages the interface draws.
const WORDS = {
  zh: {
    edit: "编辑",
    undo: "撤销",
    redo: "重做",
    cut: "剪切",
    copy: "复制",
    paste: "粘贴",
    selectAll: "全选",
  },
  en: {
    edit: "Edit",
    undo: "Undo",
    redo: "Redo",
    cut: "Cut",
    copy: "Copy",
    paste: "Paste",
    selectAll: "Select All",
  },
};

function wordsFor(lang) {
  return WORDS[lang] ?? WORDS.en;
}

// The edit commands as roles carrying explicit labels: roles do the clipboard
// work, the labels follow the interface language.
function editItems(lang, can) {
  const w = wordsFor(lang);
  const on = (flag) => (can ? { enabled: !!can[flag] } : {});
  return [
    { role: "undo", label: w.undo, ...on("canUndo") },
    { role: "redo", label: w.redo, ...on("canRedo") },
    { type: "separator" },
    { role: "cut", label: w.cut, ...on("canCut") },
    { role: "copy", label: w.copy, ...on("canCopy") },
    { role: "paste", label: w.paste, ...on("canPaste") },
    { type: "separator" },
    { role: "selectAll", label: w.selectAll, ...on("canSelectAll") },
  ];
}

// The context menu's shape, kept apart from the platform so it can be checked
// without one. editFlags is the page's own account of what is possible at the
// click: this only decides what to offer.
function contextTemplate(params, lang) {
  if (!params.isEditable && !params.selectionText) return [];
  return editItems(lang, params.editFlags ?? {});
}

function editMenuTemplate(lang) {
  return { label: wordsFor(lang).edit, submenu: editItems(lang) };
}

module.exports = { contextTemplate, editMenuTemplate };
