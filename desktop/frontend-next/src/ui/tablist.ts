import type { KeyboardEvent } from "react";

// A tablist that does not answer the arrow keys is a tablist in name only, and
// the two in this app must not behave differently.
export function arrowTabs(e: KeyboardEvent<HTMLElement>) {
  const tabs = [...e.currentTarget.querySelectorAll<HTMLElement>('[role="tab"]')];
  const i = tabs.indexOf(document.activeElement as HTMLElement);
  if (i < 0) return;
  const to = e.key === "ArrowRight" || e.key === "ArrowDown" ? i + 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? i - 1 : -1;
  if (to < 0 || to >= tabs.length) return;
  e.preventDefault();
  tabs[to].focus();
  tabs[to].click();
}
