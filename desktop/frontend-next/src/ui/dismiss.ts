import { useEffect } from "react";
import type { RefObject } from "react";

// Closes a popover on an outside press or Escape. Both pickers in the model
// pane use it so a menu cannot be left hanging open behind the next click, and
// so neither has to reimplement the listener teardown.
//
// `also` is a second box that counts as inside. A menu rendered through a portal
// is not a DOM descendant of its trigger, so without it every click on the menu
// reads as a click away and closes the thing being clicked.
export function useDismiss(
  open: boolean,
  box: RefObject<HTMLElement | null>,
  close: () => void,
  also?: RefObject<HTMLElement | null>,
) {
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      const t = e.target as Node;
      if (!box.current?.contains(t) && !also?.current?.contains(t)) close();
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
  }, [open, box, close, also]);
}
