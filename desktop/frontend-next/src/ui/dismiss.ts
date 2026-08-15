import { useEffect } from "react";
import type { RefObject } from "react";

// Closes a popover on an outside press or Escape. Both pickers in the model
// pane use it so a menu cannot be left hanging open behind the next click, and
// so neither has to reimplement the listener teardown.
export function useDismiss(open: boolean, box: RefObject<HTMLElement | null>, close: () => void) {
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) close();
    };
    const esc = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      // The pane closes on Escape too; without this the menu and the whole
      // settings pane would both go on one press.
      e.stopPropagation();
      close();
    };
    addEventListener("mousedown", away);
    addEventListener("keydown", esc, true);
    return () => {
      removeEventListener("mousedown", away);
      removeEventListener("keydown", esc, true);
    };
  }, [open, box, close]);
}
