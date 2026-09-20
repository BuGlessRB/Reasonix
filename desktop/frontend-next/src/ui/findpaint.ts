// Ranges, not wrapper elements: marking matches by editing the tree would mean
// rewriting markdown React owns, inside cards that re-render on every delta.
const ALL = "rx-find";
const NOW = "rx-find-now";

// Absent on older engines. The count and the scroll still work without it, so
// what is lost is the paint and nothing else.
const registry = (): HighlightRegistry | null =>
  typeof CSS !== "undefined" && "highlights" in CSS && typeof Highlight === "function" ? CSS.highlights : null;

export function clearFind(): void {
  const reg = registry();
  if (!reg) return;
  reg.delete(ALL);
  reg.delete(NOW);
}

/** Paints occurrences under `root`, with row `current` painted as the one being
 *  looked at. Only mounted rows can be painted; the count speaks for the rest. */
export function paintFind(root: HTMLElement | null, query: string, current: string | null): void {
  const reg = registry();
  if (!reg) return;
  const needle = query.trim().toLocaleLowerCase();
  if (!root || !needle) return clearFind();

  const rest: Range[] = [];
  const now: Range[] = [];
  for (const row of root.querySelectorAll<HTMLElement>("[data-item]")) {
    const into = row.dataset.item === current ? now : rest;
    const walk = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
    for (let node = walk.nextNode(); node; node = walk.nextNode()) {
      const hay = (node.nodeValue ?? "").toLocaleLowerCase();
      // A range is addressed in the original's units, and lowercasing changes
      // length in some scripts. Skipping leaves the hit unpainted, never wrong.
      if (hay.length !== (node.nodeValue ?? "").length) continue;
      for (let at = hay.indexOf(needle); at >= 0; at = hay.indexOf(needle, at + needle.length)) {
        const range = document.createRange();
        range.setStart(node, at);
        range.setEnd(node, at + needle.length);
        into.push(range);
      }
    }
  }
  reg.set(ALL, new Highlight(...rest));
  reg.set(NOW, new Highlight(...now));
}
